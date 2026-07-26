package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type BackendConfig struct {
	Addr string
	Role string
}

type Config struct {
	ListenAddr  string
	Backends    []BackendConfig
	LoadBalance string
	Verbose     bool
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := &Config{
		ListenAddr:  "127.0.0.1:16379", // default fallback
		LoadBalance: "random",          // default strategy
		Verbose:     false,             // default mode
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Split on the first '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line %d: invalid format, expected key = value", lineNum)
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "listen_addr":
			cfg.ListenAddr = val
		case "load_balance":
			cfg.LoadBalance = val
		case "verbose":
			cfg.Verbose = (val == "true")
		case "backend":
			// Backend format: "addr role"
			backendParts := strings.Fields(val)
			if len(backendParts) != 2 {
				return nil, fmt.Errorf("line %d: invalid backend format, expected 'addr role'", lineNum)
			}
			cfg.Backends = append(cfg.Backends, BackendConfig{
				Addr: backendParts[0],
				Role: backendParts[1],
			})
		default:
			return nil, fmt.Errorf("line %d: unknown configuration key %q", lineNum, key)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(cfg.Backends) == 0 {
		return nil, fmt.Errorf("no backend servers configured")
	}

	return cfg, nil
}
