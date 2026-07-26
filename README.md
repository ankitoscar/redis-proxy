# Redis Read-Write Splitting Proxy

A high-performance Redis routing middleware built in Go. It parses commands using the Redis Serialization Protocol (RESP), identifies mutating (write) vs. query (read) commands, and dynamically routes traffic to Master or Replica instances.

---

## Features

- **RESP Parsing & Serialization**: Custom built RESP parser and serializer supporting Simple Strings, Errors, Integers, Bulk Strings, and Arrays.
- **Read/Write Splitting**: Mutating commands (e.g. `SET`, `DEL`, `LPUSH`, `HSET`) are automatically routed to the Master. Read-only queries (e.g. `GET`, `PING`) are routed to a Replica (falling back to Master if no replicas are available).
- **Dynamic Configuration**: Supports specifying the proxy listener port and backend server topologies in a `.conf` file without modifying the code.
- **Docker Compose Setup**: Quick local testing stack containing 1 Master Redis and 1 Replica Redis.
- **Integration Test Runner**: Clean bash script to orchestrate and verify the end-to-end routing pipeline with detailed logs on failure.

---

## Directory Structure

```text
├── backend/             # Upstream connection client
├── config/              # Configuration loader module
├── parser/              # RESP Protocol Parser and Command Classifier
├── server/              # Proxy Server TCP listener and Router
├── docker-compose.yml   # Multi-instance testing environment (Master + Replica)
├── go.mod               # Go module declaration
├── main.go              # Proxy entry point
├── redis-proxy.conf     # Default configuration parameters
└── test_integration.sh  # Automated integration test runner
```

---

## Getting Started

### Prerequisites
- Go 1.20+
- Docker & Docker Compose
- `redis-tools` (for `redis-cli`)

### 1. Configuration (`redis-proxy.conf`)
You can adjust listener addresses and backend targets using the config file:
```conf
# Address where proxy listener will bind
listen_addr = 127.0.0.1:16379

# Backend servers: backend = <address> <role>
backend = 127.0.0.1:6379 master
backend = 127.0.0.1:6380 replica

# Logging level: verbose = true (logs connection details and backend routing) or false (logs only command and status)
verbose = false
```

### 2. Running Local Redis Instances
Spin up the Redis Master (port `6379`) and Replica (port `6380` replicating master) stack:
```bash
docker compose up -d
```

### 3. Build & Run the Proxy
Build the proxy server:
```bash
go build -o redis-proxy .
```

Start the proxy in the foreground:
```bash
./redis-proxy start -config redis-proxy.conf
```

Start the proxy in the background (daemon mode):
```bash
./redis-proxy start -daemon -config redis-proxy.conf
```

Stop the running proxy daemon:
```bash
./redis-proxy stop
```

Reload the configuration (zero-downtime hot-reload):
```bash
./redis-proxy reload
```

Verify configuration file validity:
```bash
./redis-proxy check -config redis-proxy.conf
```

### 4. Interacting with the Proxy
Connect using `redis-cli` on port `16379`:
```bash
redis-cli -h 127.0.0.1 -p 16379
```
Try running some commands:
```redis
127.0.0.1:16379> SET proxytest "hello master"
OK
127.0.0.1:16379> GET proxytest
"hello master"
```

---

## Verification & Testing

### Unit Tests
Run all unit tests in the codebase:
```bash
go test -v ./...
```

### Integration Tests
Run the automated end-to-end integration test. It spins up the Docker containers, waits for replica replication sync, runs sample requests, verifies the split routing in the logs, and tears down the containers on completion:
```bash
./test_integration.sh
```
