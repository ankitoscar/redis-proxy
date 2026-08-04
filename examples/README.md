# Redis Proxy Client Examples

This directory contains examples of connecting to the Redis Proxy using official and community-standard Redis clients in different programming languages:
- **Python**: Using the `redis` library.
- **Go**: Using `go-redis`.
- **JavaScript (Node.js)**: Using the `redis` npm package.

## How the Proxy Works with Clients

1. **Host and Port**: By default, the Redis Proxy listens on `127.0.0.1:16379`. Point your Redis client configurations to this address.
2. **Transparent Authentication**: 
   - If your upstream Redis master and replicas require a password, the Redis Proxy is configured with that password in `redis-proxy.conf` and handles authentication to the backends automatically.
   - Therefore, your Redis client **does not** need to provide authentication credentials when connecting to the proxy (i.e. leave the `password` field blank or omit it).
3. **Read/Write Splitting & Load Balancing**:
   - Write commands (like `SET`, `DEL`, `HSET`, etc.) are automatically routed to the Master backend.
   - Read commands (like `GET`, `PING`, etc.) are automatically load-balanced across the healthy Replica backends (or Master as fallback) according to the configured strategy (`random` or `round-robin`).

---

## Python Example

### Setup
Ensure you have the `redis` client package installed:
```bash
pip install redis
```

### Run
Execute the Python example:
```bash
python python/client.py
```

---

## Go Example

### Setup
Go to the `go` directory:
```bash
cd go
go run client.go
```

---

## JavaScript (Node.js) Example

### Setup
Install the dependencies:
```bash
cd javascript
npm install
```

### Run
Execute the Node.js example:
```bash
node client.js
```
