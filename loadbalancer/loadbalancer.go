package loadbalancer

import (
	"errors"
	"math/rand"
	"redis-proxy/config"
	"sync"
	"sync/atomic"
)

type Strategy string

const (
	StrategyRandom              Strategy = "random"
	StrategyRoundRobin          Strategy = "round-robin"
	StrategyWeightedRandom      Strategy = "weighted-random"
	StrategyWeightedRoundRobin  Strategy = "weighted-round-robin"
	StrategyWeighted            Strategy = "weighted" // alias for weighted-round-robin
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

type WeightedRandomBalancer struct{}

func (w *WeightedRandomBalancer) Select(backends []config.BackendConfig) (config.BackendConfig, error) {
	if len(backends) == 0 {
		return config.BackendConfig{}, errors.New("no backends available")
	}
	totalWeight := 0
	for _, b := range backends {
		weight := b.Weight
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}
	r := rand.Intn(totalWeight)
	for _, b := range backends {
		weight := b.Weight
		if weight <= 0 {
			weight = 1
		}
		r -= weight
		if r < 0 {
			return b, nil
		}
	}
	return backends[0], nil
}

type WeightedRoundRobinBalancer struct {
	mu             sync.Mutex
	currentWeights map[string]int
}

func NewWeightedRoundRobinBalancer() *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		currentWeights: make(map[string]int),
	}
}

func (w *WeightedRoundRobinBalancer) Select(backends []config.BackendConfig) (config.BackendConfig, error) {
	if len(backends) == 0 {
		return config.BackendConfig{}, errors.New("no backends available")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	totalWeight := 0
	maxWeight := -1 << 31
	var selected config.BackendConfig
	selectedIdx := -1

	for i, b := range backends {
		weight := b.Weight
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight

		curr := w.currentWeights[b.Addr] + weight
		w.currentWeights[b.Addr] = curr

		if curr > maxWeight {
			maxWeight = curr
			selected = b
			selectedIdx = i
		}
	}

	if selectedIdx >= 0 {
		w.currentWeights[selected.Addr] -= totalWeight
		return selected, nil
	}

	return backends[0], nil
}

func NewLoadBalancer(strategy Strategy) (LoadBalancer, error) {
	switch strategy {
	case StrategyRandom, "":
		return &RandomBalancer{}, nil
	case StrategyRoundRobin:
		return &RoundRobinBalancer{}, nil
	case StrategyWeightedRandom:
		return &WeightedRandomBalancer{}, nil
	case StrategyWeightedRoundRobin, StrategyWeighted:
		return NewWeightedRoundRobinBalancer(), nil
	default:
		return nil, errors.New("unsupported load balancing strategy: " + string(strategy))
	}
}
