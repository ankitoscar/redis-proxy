package server

import (
	"fmt"
	"net"
	"os/exec"
	"redis-proxy/config"
	"redis-proxy/parser"
	"strings"
	"testing"
	"time"
)

func runRedisCommand(addr string, password string, args ...string) (parser.Value, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return parser.Value{}, err
	}
	defer conn.Close()

	p := parser.NewParser(conn)

	// Authenticate if password is provided
	if password != "" {
		authCmd := parser.Value{
			Type: parser.TypeArray,
			Array: []parser.Value{
				{Type: parser.TypeBulkString, Str: "AUTH"},
				{Type: parser.TypeBulkString, Str: password},
			},
		}
		if err := parser.WriteValue(conn, authCmd); err != nil {
			return parser.Value{}, err
		}
		resp, err := p.Read()
		if err != nil {
			return parser.Value{}, err
		}
		if resp.Type == parser.TypeSimpleString && strings.ToUpper(resp.Str) != "OK" {
			return parser.Value{}, fmt.Errorf("AUTH failed: %s", resp.Str)
		}
	}

	// Send actual command
	var valArgs []parser.Value
	for _, arg := range args {
		valArgs = append(valArgs, parser.Value{Type: parser.TypeBulkString, Str: arg})
	}
	cmd := parser.Value{
		Type:  parser.TypeArray,
		Array: valArgs,
	}
	if err := parser.WriteValue(conn, cmd); err != nil {
		return parser.Value{}, err
	}

	return p.Read()
}

func sendProxyCmd(conn net.Conn, p *parser.Parser, args ...string) (parser.Value, error) {
	var valArgs []parser.Value
	for _, arg := range args {
		valArgs = append(valArgs, parser.Value{Type: parser.TypeBulkString, Str: arg})
	}
	cmd := parser.Value{
		Type:  parser.TypeArray,
		Array: valArgs,
	}
	if err := parser.WriteValue(conn, cmd); err != nil {
		return parser.Value{}, err
	}
	return p.Read()
}

func TestEndToEnd(t *testing.T) {
	// 1. Start Redis Cluster (1 Master + 3 Replicas) using Docker Compose
	t.Log("=== 1. Starting Redis Containers ===")
	upCmd := exec.Command("docker", "compose", "-f", "../tests/docker-compose-e2e.yml", "up", "-d")
	if output, err := upCmd.CombinedOutput(); err != nil {
		t.Logf("Warning: failed to start redis containers: %v (Output: %s). Assuming they are already running.", err, string(output))
	}

	// Defer cleanup of containers
	defer func() {
		t.Log("=== Cleaning up Redis Containers ===")
		downCmd := exec.Command("docker", "compose", "-f", "../tests/docker-compose-e2e.yml", "down")
		if output, err := downCmd.CombinedOutput(); err != nil {
			t.Logf("Warning: failed to clean up containers: %v (Output: %s)", err, string(output))
		}
	}()

	// 2. Wait for Redis instances to respond to PING
	t.Log("=== 2. Waiting for Redis instances to accept connections ===")
	ports := []string{"6379", "6380", "6381", "6382"}
	password := "e2e_secret_password"

	for _, port := range ports {
		addr := "127.0.0.1:" + port
		ready := false
		for i := 0; i < 30; i++ {
			resp, err := runRedisCommand(addr, password, "PING")
			if err == nil {
				if resp.Type == parser.TypeSimpleString && strings.ToUpper(resp.Str) == "PONG" {
					ready = true
					break
				}
				if resp.Type == parser.TypeBulkString && strings.ToUpper(resp.Str) == "PONG" {
					ready = true
					break
				}
			}
			time.Sleep(1 * time.Second)
		}
		if !ready {
			t.Fatalf("Redis instance at %s did not become ready", addr)
		}
	}
	t.Log("All Redis instances are responding to PING.")

	// 3. Wait for Replicas to establish link with Master
	t.Log("=== 3. Waiting for replication handshake to complete ===")
	replicaPorts := []string{"6380", "6381", "6382"}
	for _, port := range replicaPorts {
		addr := "127.0.0.1:" + port
		linkUp := false
		for i := 0; i < 30; i++ {
			resp, err := runRedisCommand(addr, password, "INFO", "replication")
			if err == nil && strings.Contains(resp.Str, "master_link_status:up") {
				linkUp = true
				break
			}
			time.Sleep(1 * time.Second)
		}
		if !linkUp {
			t.Fatalf("Replica at %s did not synchronize with master", addr)
		}
	}
	t.Log("All replica links are UP and synchronized!")

	// 4. Configure and start Proxy Server on a dynamic port
	t.Log("=== 4. Starting Proxy Server ===")
	backendConfigs := []config.BackendConfig{
		{Addr: "127.0.0.1:6379", Role: "master", Weight: 1},
		{Addr: "127.0.0.1:6380", Role: "replica", Weight: 5},
		{Addr: "127.0.0.1:6381", Role: "replica", Weight: 1},
		{Addr: "127.0.0.1:6382", Role: "replica", Weight: 1},
	}

	srv, err := NewServer("127.0.0.1:0", backendConfigs, "weighted-round-robin", true, "", password)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	proxyAddr := proxyListener.Addr().String()
	t.Logf("Proxy listening dynamically on %s", proxyAddr)

	// Start accept loop
	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go srv.handleConnection(conn)
		}
	}()

	// Mark all backends healthy in proxy state
	srv.mu.Lock()
	for _, cfg := range backendConfigs {
		srv.backendsHealth[cfg.Addr] = true
	}
	srv.mu.Unlock()

	// 5. Connect Client & Execute E2E Simulation
	t.Log("=== 5. Starting Client Simulation ===")
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	p := parser.NewParser(conn)

	keys := []string{"e2e_key_1", "e2e_key_2", "e2e_key_3"}

	// Run loop for 5 seconds, updating values every 500ms
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(5 * time.Second)
	iteration := 0

	for {
		select {
		case <-timeout:
			t.Log("E2E simulation finished successfully.")
			return
		case <-ticker.C:
			iteration++
			t.Logf("--- Iteration %d ---", iteration)

			for _, key := range keys {
				val := fmt.Sprintf("val_iter_%d_%s", iteration, key)

				// 1. SET key val (routes to master)
				t.Logf("SET %s = %s", key, val)
				setResp, err := sendProxyCmd(conn, p, "SET", key, val)
				if err != nil {
					t.Fatalf("SET failed: %v", err)
				}
				if setResp.Type != parser.TypeSimpleString || strings.ToUpper(setResp.Str) != "OK" {
					t.Fatalf("Expected OK for SET response, got: %s", setResp.Str)
				}

				// 2. Allow short replication delay (100ms) for write to propagate to replicas
				time.Sleep(100 * time.Millisecond)

				// 3. GET key (routes to replicas)
				t.Logf("GET %s", key)
				getResp, err := sendProxyCmd(conn, p, "GET", key)
				if err != nil {
					t.Fatalf("GET failed: %v", err)
				}
				if getResp.Type != parser.TypeBulkString || getResp.Str != val {
					t.Fatalf("Expected GET to return %q, got: %q", val, getResp.Str)
				}
				t.Logf("Successfully retrieved expected value: %s", getResp.Str)
			}
		}
	}
}
