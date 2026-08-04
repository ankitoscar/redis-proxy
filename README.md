# Redis Read-Write Splitting & Load-Balancing Proxy

A high-performance Redis routing middleware built in Go. It parses commands using the Redis Serialization Protocol (RESP), identifies mutating (write) vs. query (read) commands, dynamically load-balances reads across healthy replicas, and supports zero-downtime hot-reloading.

---

## Features

- **RESP Parsing & Serialization**: Custom-built RESP parser and serializer supporting Simple Strings, Errors, Integers, Bulk Strings, and Arrays.
- **Read/Write Splitting**: Mutating commands (e.g., `SET`, `DEL`, `LPUSH`, `HSET`) are automatically routed to the Master. Read-only queries (e.g., `GET`, `PING`) are routed to Replicas (falling back to Master if no replicas are online).
- **Request-Level Load Balancing**: Supports distributing read commands dynamically *per request* instead of per connection. Includes two load-balancing strategies:
  - `random`: Randomly selects from the pool of healthy read backends.
  - `round-robin`: Rotates sequentially through healthy read backends.
- **Lazy Connection Management**: Lazily establishes and caches TCP connections to replica backends only when they are selected, optimizing resources.
- **Dynamic Configuration & Hot-Reloading**: Supports zero-downtime config hot-reloading via `./redis-proxy reload` (using Unix `SIGHUP` signal), allowing strategy switches or backend list changes on the fly.
- **Authentication & ACLs**: Authenticates connections to Redis backend instances with passwords and optional usernames (for Redis 6+ Access Control Lists).
- **Health Checking**: Background loop automatically runs health checks against all backends, marking them online or offline dynamically.
- **Continuous Integration (CI/CD)**: GitHub Actions workflow to verify formatting, build binaries, run unit/integration tests, and publish cross-platform releases (`linux/amd64` and `darwin/amd64`) on push of a tag (`v*`).

---

## Directory Structure

```text
├── backend/               # Upstream TCP connection client
├── config/                # Configuration parser module
├── loadbalancer/          # Load-balancing strategy algorithms (random, round-robin)
├── parser/                # RESP Protocol Parser and Command Classifier
├── server/                # Proxy Server TCP listener and Router
├── tests/
│   ├── docker-compose.yml # Multi-instance Redis stack (1 Master, 2 Replicas with Password Auth)
│   └── test_integration.sh# Automated integration test runner
├── .github/
│   └── workflows/
│       └── ci.yml         # GitHub Actions CI/CD Pipeline
├── go.mod                 # Go module declaration
├── main.go                # CLI Daemon entry point
└── redis-proxy.conf       # Configuration file template
```

---

## Getting Started

### Prerequisites
- Go 1.21+
- Docker & Docker Compose
- `redis-tools` (for local verification using `redis-cli`)

### 1. Configuration (`redis-proxy.conf`)
Adjust proxy listener binding, backend targets, and authentication details:
```ini
# Address where proxy listener will bind
listen_addr = 127.0.0.1:16379

# Backend servers: backend = <address> <role>
backend = 127.0.0.1:6379 master
backend = 127.0.0.1:6380 replica
backend = 127.0.0.1:6381 replica

# Username for authenticating with Redis backends (optional, required if using ACLs)
# username = myusername

# Password for authenticating with Redis backends
password = mysecretpassword

# Load balancing strategy for reads: random or round-robin
load_balance = round-robin

# Logging level: verbose = true (logs connection details and backend routing) or false
verbose = true
```

### 2. Build & Run the Proxy CLI

Build the proxy binary:
```bash
go build -o redis-proxy .
```

Validate your configuration file:
```bash
./redis-proxy check -config redis-proxy.conf
```

Start the proxy server in the foreground:
```bash
./redis-proxy start -config redis-proxy.conf
```

Start the proxy in daemon mode (background):
```bash
./redis-proxy start -daemon -config redis-proxy.conf
```

Trigger zero-downtime hot-reload:
```bash
./redis-proxy reload
```

Stop the running proxy daemon:
```bash
./redis-proxy stop
```

### 3. Running with Docker (Multi-Stage Build)

We also provide a multi-stage `Dockerfile` to build and run the Redis Proxy in a containerized environment.

#### Build the Docker image:
```bash
docker build -t redis-proxy:latest .
```

#### Run the configuration check inside the container:
```bash
docker run --rm -v $(pwd)/redis-proxy.conf:/etc/redis-proxy/redis-proxy.conf redis-proxy:latest check -config /etc/redis-proxy/redis-proxy.conf
```

#### Run the Redis Proxy container:
> [!NOTE]
> Ensure that your config file (`redis-proxy.conf`) has `listen_addr = 0.0.0.0:16379` instead of `127.0.0.1:16379` so that it is accessible outside the container.

```bash
docker run -d --name redis-proxy \
  -p 16379:16379 \
  -v $(pwd)/redis-proxy.conf:/etc/redis-proxy/redis-proxy.conf \
  redis-proxy:latest
```

---

## Client Integration Examples

We provide code examples for integrating popular Redis client libraries with the proxy:
- **Python**: Using `redis-py`
- **Go**: Using `go-redis`
- **JavaScript**: Using `redis` (Node.js)

See the [examples/README.md](file:///media/ankit/Data/Projects/redis_proxy/examples/README.md) directory for details on how to set up and run these examples.

---

## Verification & Testing

### Unit Tests
Run all unit tests in the codebase:
```bash
go test -v ./...
```

### Integration Tests
Run the automated end-to-end integration test. It spins up the Docker containers, waits for replicas to establish links with the master, starts the proxy, runs write/read commands under both load-balancing strategies (reloading configurations dynamically), and cleans up all Docker resources on completion:
```bash
./tests/test_integration.sh
```
