package server

import (
	"io"
	"log"
	"net"
	"redis-proxy/config"
	"redis-proxy/parser"
	"testing"
)

func init() {
	log.SetOutput(io.Discard)
}

func startMockRedisServerForBench(b *testing.B, response []byte) net.Listener {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to start mock redis server: %v", err)
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

func setupBenchmarkServer(b *testing.B, lbStrategy string, numReplicas int) (*Server, []net.Listener, string) {
	// 1. Start mock master
	masterListener := startMockRedisServerForBench(b, []byte("+OK\r\n"))

	// 2. Start mock replicas
	replicaListeners := make([]net.Listener, numReplicas)
	for i := 0; i < numReplicas; i++ {
		replicaListeners[i] = startMockRedisServerForBench(b, []byte("$5\r\nvalue\r\n"))
	}

	// 3. Create backend configs
	backendConfigs := []config.BackendConfig{
		{Addr: masterListener.Addr().String(), Role: "master"},
	}
	for i, rl := range replicaListeners {
		backendConfigs = append(backendConfigs, config.BackendConfig{
			Addr: rl.Addr().String(), Role: "replica", Weight: i + 1,
		})
	}

	// 4. Create Server (verbose = false, no auth)
	srv, err := NewServer("127.0.0.1:0", backendConfigs, lbStrategy, false, "", "")
	if err != nil {
		b.Fatalf("failed to create proxy server: %v", err)
	}

	// 5. Mark all healthy
	srv.mu.Lock()
	for _, cfg := range backendConfigs {
		srv.backendsHealth[cfg.Addr] = true
	}
	srv.mu.Unlock()

	// 6. Start proxy listener
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to start proxy listener: %v", err)
	}

	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go srv.handleConnection(conn)
		}
	}()

	listeners := append([]net.Listener{masterListener, proxyListener}, replicaListeners...)
	return srv, listeners, proxyListener.Addr().String()
}

func BenchmarkProxy_Read_Random_3Replicas(b *testing.B) {
	_, listeners, proxyAddr := setupBenchmarkServer(b, "random", 3)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			b.Error(err)
			return
		}
		defer conn.Close()

		cmd := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
		buf := make([]byte, 1024)
		for pb.Next() {
			_, err := conn.Write(cmd)
			if err != nil {
				return
			}
			_, err = conn.Read(buf)
			if err != nil {
				return
			}
		}
	})
}

func BenchmarkProxy_Read_RoundRobin_3Replicas(b *testing.B) {
	_, listeners, proxyAddr := setupBenchmarkServer(b, "round-robin", 3)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			b.Error(err)
			return
		}
		defer conn.Close()

		cmd := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
		buf := make([]byte, 1024)
		for pb.Next() {
			_, err := conn.Write(cmd)
			if err != nil {
				return
			}
			_, err = conn.Read(buf)
			if err != nil {
				return
			}
		}
	})
}

func BenchmarkProxy_Read_WeightedRandom_3Replicas(b *testing.B) {
	_, listeners, proxyAddr := setupBenchmarkServer(b, "weighted-random", 3)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			b.Error(err)
			return
		}
		defer conn.Close()

		cmd := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
		buf := make([]byte, 1024)
		for pb.Next() {
			_, err := conn.Write(cmd)
			if err != nil {
				return
			}
			_, err = conn.Read(buf)
			if err != nil {
				return
			}
		}
	})
}

func BenchmarkProxy_Read_WeightedRoundRobin_3Replicas(b *testing.B) {
	_, listeners, proxyAddr := setupBenchmarkServer(b, "weighted-round-robin", 3)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			b.Error(err)
			return
		}
		defer conn.Close()

		cmd := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
		buf := make([]byte, 1024)
		for pb.Next() {
			_, err := conn.Write(cmd)
			if err != nil {
				return
			}
			_, err = conn.Read(buf)
			if err != nil {
				return
			}
		}
	})
}

func BenchmarkProxy_Write(b *testing.B) {
	_, listeners, proxyAddr := setupBenchmarkServer(b, "random", 3)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			b.Error(err)
			return
		}
		defer conn.Close()

		cmd := []byte("*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n")
		buf := make([]byte, 1024)
		for pb.Next() {
			_, err := conn.Write(cmd)
			if err != nil {
				return
			}
			_, err = conn.Read(buf)
			if err != nil {
				return
			}
		}
	})
}
