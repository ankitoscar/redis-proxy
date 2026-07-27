package server

import (
	"net"
	"redis-proxy/config"
	"redis-proxy/parser"
	"strings"
	"testing"
	"time"
)

func startMockRedisServer(t *testing.T, response []byte) net.Listener {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis server: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				defer c.Close()
				p := parser.NewParser(c)
				for {
					_, err := p.Read()
					if err != nil {
						return
					}
					_, err = c.Write(response)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	return listener
}

func TestCheckBackendHealth(t *testing.T) {
	srv := &Server{password: ""}

	// Test case 1: Healthy server responding with +PONG
	l1 := startMockRedisServer(t, []byte("+PONG\r\n"))
	defer l1.Close()

	if !srv.checkBackendHealth(l1.Addr().String()) {
		t.Errorf("expected health check to succeed for +PONG response")
	}

	// Test case 2: Healthy server responding with $4\r\nPONG\r\n (bulk string)
	l2 := startMockRedisServer(t, []byte("$4\r\nPONG\r\n"))
	defer l2.Close()

	if !srv.checkBackendHealth(l2.Addr().String()) {
		t.Errorf("expected health check to succeed for bulk string PONG response")
	}

	// Test case 3: Unhealthy server responding with an error
	l3 := startMockRedisServer(t, []byte("-ERR mock error\r\n"))
	defer l3.Close()

	if srv.checkBackendHealth(l3.Addr().String()) {
		t.Errorf("expected health check to fail for error response")
	}

	// Test case 4: Server port offline
	if srv.checkBackendHealth("127.0.0.1:9999") {
		t.Errorf("expected health check to fail for offline port")
	}
}

func TestServerHealthCheckingLoop(t *testing.T) {
	l := startMockRedisServer(t, []byte("+PONG\r\n"))
	// We will close l manually in the middle of the test

	backendConfigs := []config.BackendConfig{
		{Addr: l.Addr().String(), Role: "replica"},
	}

	srv, err := NewServer("127.0.0.1:16381", backendConfigs, "random", true, "")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Initially all backends are marked unhealthy
	srv.mu.RLock()
	health := srv.backendsHealth[l.Addr().String()]
	srv.mu.RUnlock()
	if health {
		t.Errorf("expected backend to be initially unhealthy")
	}

	// Trigger manual health check
	srv.checkAllBackends()

	// Wait briefly for check to finish
	time.Sleep(100 * time.Millisecond)

	srv.mu.RLock()
	health = srv.backendsHealth[l.Addr().String()]
	srv.mu.RUnlock()
	if !health {
		t.Errorf("expected backend to become healthy after check")
	}

	// Close mock server and recheck
	l.Close()
	srv.checkAllBackends()

	time.Sleep(100 * time.Millisecond)

	srv.mu.RLock()
	health = srv.backendsHealth[l.Addr().String()]
	srv.mu.RUnlock()
	if health {
		t.Errorf("expected backend to become unhealthy after mock server closed")
	}
}

func startMockRedisServerWithCounter(t *testing.T, response []byte) (net.Listener, *int) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis server: %v", err)
	}

	counter := 0
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				p := parser.NewParser(c)
				for {
					val, err := p.Read()
					if err != nil {
						return
					}
					isPing := false
					if val.Type == parser.TypeArray && len(val.Array) > 0 && val.Array[0].Type == parser.TypeBulkString {
						if strings.ToUpper(val.Array[0].Str) == "PING" {
							isPing = true
						}
					} else if val.Type == parser.TypeBulkString && strings.ToUpper(val.Str) == "PING" {
						isPing = true
					} else if val.Type == parser.TypeSimpleString && strings.ToUpper(val.Str) == "PING" {
						isPing = true
					}

					if !isPing {
						counter++
					}
					_, err = c.Write(response)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	return listener, &counter
}

func TestServerLoadBalancing(t *testing.T) {
	response := []byte("$5\r\nvalue\r\n")

	m1, c1 := startMockRedisServerWithCounter(t, response)
	defer m1.Close()
	m2, c2 := startMockRedisServerWithCounter(t, response)
	defer m2.Close()

	backendConfigs := []config.BackendConfig{
		{Addr: m1.Addr().String(), Role: "master"},
		{Addr: m2.Addr().String(), Role: "replica"},
	}

	srv, err := NewServer("127.0.0.1:0", backendConfigs, "random", true, "")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.mu.Lock()
	srv.backendsHealth[m1.Addr().String()] = true
	srv.backendsHealth[m2.Addr().String()] = true
	srv.mu.Unlock()

	for i := 0; i < 50; i++ {
		clientConn, proxyConn := net.Pipe()
		go srv.handleConnection(proxyConn)

		_, err := clientConn.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"))
		if err != nil {
			t.Fatalf("failed to write to client: %v", err)
		}

		buf := make([]byte, 1024)
		_, err = clientConn.Read(buf)
		if err != nil {
			t.Fatalf("failed to read from client: %v", err)
		}

		clientConn.Close()
	}

	totalCalls := *c1 + *c2
	if totalCalls != 50 {
		t.Errorf("expected 50 total calls, got %d", totalCalls)
	}

	t.Logf("Calls routed to Master (m1): %d, Calls routed to Replica (m2): %d", *c1, *c2)

	if *c1 < 10 || *c2 < 10 {
		t.Errorf("expected random load distribution, got master calls: %d, replica calls: %d", *c1, *c2)
	}
}

func TestServerLoadBalancingRoundRobin(t *testing.T) {
	response := []byte("$5\r\nvalue\r\n")

	m1, c1 := startMockRedisServerWithCounter(t, response)
	defer m1.Close()
	m2, c2 := startMockRedisServerWithCounter(t, response)
	defer m2.Close()

	backendConfigs := []config.BackendConfig{
		{Addr: m1.Addr().String(), Role: "master"},
		{Addr: m2.Addr().String(), Role: "replica"},
	}

	srv, err := NewServer("127.0.0.1:0", backendConfigs, "round-robin", true, "")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.mu.Lock()
	srv.backendsHealth[m1.Addr().String()] = true
	srv.backendsHealth[m2.Addr().String()] = true
	srv.mu.Unlock()

	for i := 0; i < 10; i++ {
		clientConn, proxyConn := net.Pipe()
		go srv.handleConnection(proxyConn)

		_, err := clientConn.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"))
		if err != nil {
			t.Fatalf("failed to write to client: %v", err)
		}

		buf := make([]byte, 1024)
		_, err = clientConn.Read(buf)
		if err != nil {
			t.Fatalf("failed to read from client: %v", err)
		}

		clientConn.Close()
	}

	if *c1 != 5 || *c2 != 5 {
		t.Errorf("expected round-robin to split exactly 5/5, got master: %d, replica: %d", *c1, *c2)
	}
}
