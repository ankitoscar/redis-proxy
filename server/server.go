package server

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"redis-proxy/backend"
	"redis-proxy/config"
	"redis-proxy/loadbalancer"
	"redis-proxy/parser"
)

type Server struct {
	addr           string
	backendConfigs []config.BackendConfig
	mu             sync.RWMutex
	backendsHealth map[string]bool
	lb             loadbalancer.LoadBalancer
	verbose        bool
}

func NewServer(addr string, backendConfigs []config.BackendConfig, lbStrategy string, verbose bool) (*Server, error) {
	backendsHealth := make(map[string]bool)
	for _, cfg := range backendConfigs {
		backendsHealth[cfg.Addr] = false
	}

	lb, err := loadbalancer.NewLoadBalancer(loadbalancer.Strategy(lbStrategy))
	if err != nil {
		return nil, err
	}

	return &Server{
		addr:           addr,
		backendConfigs: backendConfigs,
		backendsHealth: backendsHealth,
		lb:             lb,
		verbose:        verbose,
	}, nil
}

func (s *Server) Start() error {
	// Start health checker loop (every 2 seconds)
	go s.runHealthChecker(2 * time.Second)

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Redis proxy listening on %s", s.addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) runHealthChecker(interval time.Duration) {
	s.checkAllBackends()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		s.checkAllBackends()
	}
}

func (s *Server) checkAllBackends() {
	s.mu.RLock()
	configs := make([]config.BackendConfig, len(s.backendConfigs))
	copy(configs, s.backendConfigs)
	s.mu.RUnlock()

	for _, cfg := range configs {
		go func(addr string) {
			online := checkBackendHealth(addr)
			s.mu.Lock()
			prev := s.backendsHealth[addr]
			s.backendsHealth[addr] = online
			s.mu.Unlock()

			if prev != online {
				if s.verbose {
					log.Printf("Backend %s health status changed: online=%t", addr, online)
				}
			}
		}(cfg.Addr)
	}
}

func checkBackendHealth(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(1 * time.Second))

	pingCmd := parser.Value{
		Type: parser.TypeArray,
		Array: []parser.Value{
			{Type: parser.TypeBulkString, Str: "PING"},
		},
	}

	err = parser.WriteValue(conn, pingCmd)
	if err != nil {
		return false
	}

	p := parser.NewParser(conn)
	resp, err := p.Read()
	if err != nil {
		return false
	}

	if resp.Type == parser.TypeSimpleString && strings.ToUpper(resp.Str) == "PONG" {
		return true
	}
	if resp.Type == parser.TypeBulkString && strings.ToUpper(resp.Str) == "PONG" {
		return true
	}

	return false
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	if s.verbose {
		log.Printf("New client connection from: %s", conn.RemoteAddr())
	}

	s.mu.RLock()
	var healthyMasters []config.BackendConfig
	var healthyBackends []config.BackendConfig
	for _, cfg := range s.backendConfigs {
		if s.backendsHealth[cfg.Addr] {
			healthyBackends = append(healthyBackends, cfg)
			if strings.ToLower(cfg.Role) == "master" {
				healthyMasters = append(healthyMasters, cfg)
			}
		}
	}

	var masterCfg config.BackendConfig
	if len(healthyMasters) > 0 {
		masterCfg = healthyMasters[0]
	} else {
		// Last resort fallback
		for _, cfg := range s.backendConfigs {
			if strings.ToLower(cfg.Role) == "master" {
				masterCfg = cfg
				break
			}
		}
	}

	var replicaCfg config.BackendConfig
	if len(healthyBackends) > 0 {
		var err error
		replicaCfg, err = s.lb.Select(healthyBackends)
		if err != nil {
			replicaCfg = masterCfg
		}
	} else {
		replicaCfg = masterCfg
	}
	s.mu.RUnlock()

	if s.verbose {
		log.Printf("Routing for client %s: Master=%s, Replica=%s", conn.RemoteAddr(), masterCfg.Addr, replicaCfg.Addr)
	}

	masterClient, err := backend.Connect(masterCfg.Addr)
	if err != nil {
		log.Printf("Failed to connect to master backend %s: %v", masterCfg.Addr, err)
		return
	}
	defer masterClient.Close()

	var replicaClient *backend.Client
	if replicaCfg.Addr == masterCfg.Addr {
		replicaClient = masterClient
	} else {
		replicaClient, err = backend.Connect(replicaCfg.Addr)
		if err != nil {
			log.Printf("Failed to connect to replica backend %s: %v", replicaCfg.Addr, err)
			return
		}
		defer replicaClient.Close()
	}

	p := parser.NewParser(conn)
	for {
		val, err := p.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if s.verbose {
					log.Printf("Client disconnected: %s", conn.RemoteAddr())
				}
				return
			}
			if s.verbose {
				log.Printf("Error reading from client %s: %v", conn.RemoteAddr(), err)
			}
			return
		}

		cmdStr := formatCommand(val)
		if s.verbose {
			log.Printf("Received from client %s: %+v", conn.RemoteAddr(), val)
		}

		var activeClient *backend.Client
		var destName string
		if parser.IsWriteCommand(val) {
			activeClient = masterClient
			destName = "Master (" + masterCfg.Addr + ")"
		} else {
			activeClient = replicaClient
			destName = "Replica (" + replicaCfg.Addr + ")"
		}

		if s.verbose {
			log.Printf("Routing command to %s", destName)
		}

		reply, err := activeClient.Execute(val)
		if err != nil {
			log.Printf("Command: %s, Status: FAILED (%v)", cmdStr, err)
			if s.verbose {
				log.Printf("Error executing command on backend %s: %v", destName, err)
			}
			return
		}

		log.Printf("Command: %s, Status: SUCCESS", cmdStr)
		if s.verbose {
			log.Printf("Received from backend %s: %+v", destName, reply)
		}

		err = parser.WriteValue(conn, reply)
		if err != nil {
			if s.verbose {
				log.Printf("Error writing reply to client %s: %v", conn.RemoteAddr(), err)
			}
			return
		}
	}
}

func (s *Server) Reload(backendConfigs []config.BackendConfig, lbStrategy string, verbose bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lb, err := loadbalancer.NewLoadBalancer(loadbalancer.Strategy(lbStrategy))
	if err != nil {
		return err
	}

	s.backendConfigs = backendConfigs
	s.lb = lb
	s.verbose = verbose

	newHealth := make(map[string]bool)
	for _, cfg := range backendConfigs {
		if val, exists := s.backendsHealth[cfg.Addr]; exists {
			newHealth[cfg.Addr] = val
		} else {
			newHealth[cfg.Addr] = false
		}
	}
	s.backendsHealth = newHealth

	log.Printf("Successfully reloaded server configuration. Backends count: %d, strategy: %s, verbose: %t", len(backendConfigs), lbStrategy, verbose)
	return nil
}

func formatCommand(val parser.Value) string {
	if val.Type == parser.TypeArray {
		var parts []string
		for _, item := range val.Array {
			if item.Type == parser.TypeBulkString || item.Type == parser.TypeSimpleString {
				parts = append(parts, item.Str)
			} else if item.Type == parser.TypeInteger {
				parts = append(parts, strconv.Itoa(item.Num))
			} else {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	if val.Type == parser.TypeBulkString || val.Type == parser.TypeSimpleString {
		return val.Str
	}
	if val.Type == parser.TypeInteger {
		return strconv.Itoa(val.Num)
	}
	return fmt.Sprintf("%v", val)
}

