package loadbalancer

import (
	"errors"
	"math/rand"
	"redis-proxy/config"
	"sync/atomic"
)

type Strategy string

const (
	StrategyRandom     Strategy = "random"
	StrategyRoundRobin Strategy = "round-robin"
)

type LoadBalancer interface {
	Select(backends []config.BackendConfig) (config.BackendConfig, error)
}

type RandomBalancer struct{}

func (r *RandomBalancer) Select(backends []config.BackendConfig) (config.BackendConfig, error) {
	if len(backends) == 0 {
		return config.BackendConfig{}, errors.New("no backends available")
	}
	idx := rand.Intn(len(backends))
	return backends[idx], nil
}

type RoundRobinBalancer struct {
	counter uint64
}

func (rr *RoundRobinBalancer) Select(backends []config.BackendConfig) (config.BackendConfig, error) {
	if len(backends) == 0 {
		return config.BackendConfig{}, errors.New("no backends available")
	}
	val := atomic.AddUint64(&rr.counter, 1)
	idx := int((val - 1) % uint64(len(backends)))
	return backends[idx], nil
}

func NewLoadBalancer(strategy Strategy) (LoadBalancer, error) {
	switch strategy {
	case StrategyRandom, "":
		return &RandomBalancer{}, nil
	case StrategyRoundRobin:
		return &RoundRobinBalancer{}, nil
	default:
		return nil, errors.New("unsupported load balancing strategy: " + string(strategy))
	}
}
