package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfigSuccess(t *testing.T) {
	content := `
# Simple Redis Proxy Config

listen_addr = 127.0.0.1:16379

backend = 127.0.0.1:6379 master
backend = 127.0.0.1:6380 replica
; some comment
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "proxy.conf")
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}

	expected := &Config{
		ListenAddr:  "127.0.0.1:16379",
		LoadBalance: "random",
		Backends: []BackendConfig{
			{Addr: "127.0.0.1:6379", Role: "master"},
			{Addr: "127.0.0.1:6380", Role: "replica"},
		},
	}

	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("expected %+v, got %+v", expected, cfg)
	}

	// Test case 2: Parsing explicit load_balance strategy
	contentWithLB := `
listen_addr = 127.0.0.1:16379
load_balance = round-robin
backend = 127.0.0.1:6379 master
`
	pathLB := filepath.Join(tmpDir, "proxy_lb.conf")
	err = os.WriteFile(pathLB, []byte(contentWithLB), 0644)
	if err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfgLB, err := LoadConfig(pathLB)
	if err != nil {
		t.Fatalf("failed to load valid config with load_balance: %v", err)
	}

	if cfgLB.LoadBalance != "round-robin" {
		t.Errorf("expected load_balance to be 'round-robin', got %s", cfgLB.LoadBalance)
	}

	// Test case 3: Parsing explicit verbose setting
	contentVerbose := `
listen_addr = 127.0.0.1:16379
verbose = true
backend = 127.0.0.1:6379 master
`
	pathVerbose := filepath.Join(tmpDir, "proxy_verbose.conf")
	err = os.WriteFile(pathVerbose, []byte(contentVerbose), 0644)
	if err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfgVerbose, err := LoadConfig(pathVerbose)
	if err != nil {
		t.Fatalf("failed to load valid config with verbose: %v", err)
	}

	if !cfgVerbose.Verbose {
		t.Errorf("expected verbose to be true, got false")
	}

	// Test case 4: Parsing password setting
	contentPassword := `
listen_addr = 127.0.0.1:16379
password = testsecretpassword
backend = 127.0.0.1:6379 master
`
	pathPassword := filepath.Join(tmpDir, "proxy_password.conf")
	err = os.WriteFile(pathPassword, []byte(contentPassword), 0644)
	if err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfgPassword, err := LoadConfig(pathPassword)
	if err != nil {
		t.Fatalf("failed to load valid config with password: %v", err)
	}

	if cfgPassword.Password != "testsecretpassword" {
		t.Errorf("expected password to be 'testsecretpassword', got '%s'", cfgPassword.Password)
	}

	// Test case 5: Parsing username and password settings
	contentUsername := `
listen_addr = 127.0.0.1:16379
username = testuser
password = testsecretpassword
backend = 127.0.0.1:6379 master
`
	pathUsername := filepath.Join(tmpDir, "proxy_username.conf")
	err = os.WriteFile(pathUsername, []byte(contentUsername), 0644)
	if err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfgUsername, err := LoadConfig(pathUsername)
	if err != nil {
		t.Fatalf("failed to load valid config with username: %v", err)
	}

	if cfgUsername.Username != "testuser" {
		t.Errorf("expected username to be 'testuser', got '%s'", cfgUsername.Username)
	}
	if cfgUsername.Password != "testsecretpassword" {
		t.Errorf("expected password to be 'testsecretpassword', got '%s'", cfgUsername.Password)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("non_existent_file.conf")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "Missing =",
			content: "listen_addr 127.0.0.1:16379",
		},
		{
			name:    "Unknown key",
			content: "unknown_key = value",
		},
		{
			name:    "Invalid backend format",
			content: "backend = 127.0.0.1:6379 master extra_arg",
		},
		{
			name:    "Missing role in backend",
			content: "backend = 127.0.0.1:6379",
		},
		{
			name:    "No backends defined",
			content: "listen_addr = 127.0.0.1:16379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "proxy.conf")
			err := os.WriteFile(path, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("failed to write temp config: %v", err)
			}

			_, err = LoadConfig(path)
			if err == nil {
				t.Errorf("expected error for invalid config content: %s", tt.content)
			}
		})
	}
}
