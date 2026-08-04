package loadbalancer

import (
	"redis-proxy/config"
	"testing"
)

func TestNewLoadBalancer(t *testing.T) {
	lb, err := NewLoadBalancer(StrategyRandom)
	if err != nil {
		t.Fatalf("unexpected error creating random load balancer: %v", err)
	}
	if _, ok := lb.(*RandomBalancer); !ok {
		t.Errorf("expected RandomBalancer, got %T", lb)
	}

	lb, err = NewLoadBalancer(StrategyRoundRobin)
	if err != nil {
		t.Fatalf("unexpected error creating round-robin load balancer: %v", err)
	}
	if _, ok := lb.(*RoundRobinBalancer); !ok {
		t.Errorf("expected RoundRobinBalancer, got %T", lb)
	}

	_, err = NewLoadBalancer(Strategy("invalid"))
	if err == nil {
		t.Errorf("expected error for invalid strategy, got nil")
	}
}

func TestRandomBalancer(t *testing.T) {
	lb := &RandomBalancer{}
	backends := []config.BackendConfig{
		{Addr: "127.0.0.1:6379"},
		{Addr: "127.0.0.1:6380"},
	}

	_, err := lb.Select(nil)
	if err == nil {
		t.Errorf("expected error selecting from empty slice, got nil")
	}

	// Run multiple trials to verify returned address is within bounds
	for i := 0; i < 50; i++ {
		selected, err := lb.Select(backends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected.Addr != "127.0.0.1:6379" && selected.Addr != "127.0.0.1:6380" {
			t.Errorf("unexpected selected address: %s", selected.Addr)
		}
	}
}

func TestRoundRobinBalancer(t *testing.T) {
	lb := &RoundRobinBalancer{}
	backends := []config.BackendConfig{
		{Addr: "127.0.0.1:6379"},
		{Addr: "127.0.0.1:6380"},
		{Addr: "127.0.0.1:6381"},
	}

	_, err := lb.Select(nil)
	if err == nil {
		t.Errorf("expected error selecting from empty slice, got nil")
	}

	// Verify cycle order: 6379, 6380, 6381, 6379, 6380, 6381...
	expectedSequence := []string{
		"127.0.0.1:6379",
		"127.0.0.1:6380",
		"127.0.0.1:6381",
		"127.0.0.1:6379",
		"127.0.0.1:6380",
		"127.0.0.1:6381",
	}

	for i, expected := range expectedSequence {
		selected, err := lb.Select(backends)
		if err != nil {
			t.Fatalf("trial %d: unexpected error: %v", i, err)
		}
		if selected.Addr != expected {
			t.Errorf("trial %d: expected %s, got %s", i, expected, selected.Addr)
		}
	}
}

func TestWeightedRandomBalancer(t *testing.T) {
	lb := &WeightedRandomBalancer{}
	backends := []config.BackendConfig{
		{Addr: "127.0.0.1:6379", Weight: 9},
		{Addr: "127.0.0.1:6380", Weight: 1},
	}

	_, err := lb.Select(nil)
	if err == nil {
		t.Errorf("expected error selecting from empty slice, got nil")
	}

	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		selected, err := lb.Select(backends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[selected.Addr]++
	}

	if counts["127.0.0.1:6379"] < 700 {
		t.Errorf("expected 127.0.0.1:6379 (weight 9) to be selected at least 700 times, got %d", counts["127.0.0.1:6379"])
	}
}

func TestWeightedRoundRobinBalancer(t *testing.T) {
	lb := NewWeightedRoundRobinBalancer()
	backends := []config.BackendConfig{
		{Addr: "127.0.0.1:6379", Weight: 5},
		{Addr: "127.0.0.1:6380", Weight: 1},
		{Addr: "127.0.0.1:6381", Weight: 1},
	}

	_, err := lb.Select(nil)
	if err == nil {
		t.Errorf("expected error selecting from empty slice, got nil")
	}

	expectedSequence := []string{
		"127.0.0.1:6379",
		"127.0.0.1:6379",
		"127.0.0.1:6380",
		"127.0.0.1:6379",
		"127.0.0.1:6381",
		"127.0.0.1:6379",
		"127.0.0.1:6379",
	}

	for i, expected := range expectedSequence {
		selected, err := lb.Select(backends)
		if err != nil {
			t.Fatalf("trial %d: unexpected error: %v", i, err)
		}
		if selected.Addr != expected {
			t.Errorf("trial %d: expected %s, got %s", i, expected, selected.Addr)
		}
	}
}

