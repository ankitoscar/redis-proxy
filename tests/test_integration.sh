#!/bin/bash
set -e

# File to store proxy logs during the integration test
LOG_FILE="redis-proxy.log"
rm -f "$LOG_FILE"
rm -f redis-proxy.pid

echo "=== 1. Starting Redis Docker Containers ==="
docker compose up -d

# Cleanup function to run on script exit
cleanup() {
    echo "=== Cleaning up ==="
    if [ -f redis-proxy.pid ]; then
        echo "Stopping Redis Proxy using CLI stop..."
        ./redis-proxy stop || true
    fi
    echo "Stopping Docker Containers..."
    docker compose down
}
trap cleanup EXIT

# Verbose failure handler
fail() {
    echo ""
    echo "=================================================="
    echo "=== ERROR: $1"
    echo "=================================================="
    echo ""
    echo "=== 1. Proxy Logs ($LOG_FILE) ==="
    if [ -f "$LOG_FILE" ]; then
        cat "$LOG_FILE"
    else
        echo "No proxy log file found."
    fi
    echo ""
    echo "=== 2. Docker Compose Logs ==="
    docker compose logs || true
    echo ""
    echo "=== 3. Master Replication Info ==="
    redis-cli -p 6379 info replication || true
    echo ""
    echo "=== 4. Replica Replication Info ==="
    redis-cli -p 6380 info replication || true
    exit 1
}

# Wait for master (6379) and replica (6380) to be fully ready
echo "Waiting for Redis instances to accept connections..."
until redis-cli -p 6379 PING >/dev/null 2>&1; do
    echo -n "."
    sleep 1
done
until redis-cli -p 6380 PING >/dev/null 2>&1; do
    echo -n "."
    sleep 1
done
echo " Redis instances are responding to PING."

# Wait for replication handshake to complete
echo "Waiting for Replica to establish link with Master..."
until redis-cli -p 6380 info replication | grep -q "master_link_status:up"; do
    echo -n "."
    sleep 1
done
echo " Replica link is UP and synchronized!"

echo "=== 2. Building the Redis Proxy ==="
/usr/local/go/bin/go build -o redis-proxy .

echo "=== 3. Starting the Redis Proxy CLI Daemon ==="
# Terminate any existing proxy binary running locally
pkill redis-proxy || true

# Start proxy in background using the new CLI start --daemon
./redis-proxy start -daemon -verbose -config redis-proxy.conf

# Wait a moment for PID file to be created
sleep 2

if [ ! -f redis-proxy.pid ]; then
    fail "PID file redis-proxy.pid was not created!"
fi

PROXY_PID=$(cat redis-proxy.pid)
echo "Redis Proxy started successfully with PID: $PROXY_PID"

echo "=== 4. Running Test Commands ==="
echo "Sending SET command (write) to proxy..."
SET_RESP=$(redis-cli -h 127.0.0.1 -p 16379 SET mytestkey "integration_test_passed" 2>&1) || fail "SET command failed to run"
echo "SET response: $SET_RESP"

if [ "$SET_RESP" != "OK" ]; then
    fail "SET command returned '$SET_RESP' instead of 'OK'"
fi

# Wait a short moment to ensure replication propagates
sleep 1

echo "Sending GET command (read) to proxy..."
GET_RESP=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET command failed to run"
echo "GET response: $GET_RESP"

if [ "$GET_RESP" != "integration_test_passed" ]; then
    fail "GET command returned '$GET_RESP' instead of 'integration_test_passed'"
fi

echo "=== 5. Testing CLI Reload (SIGHUP) ==="
./redis-proxy reload

# Wait a moment for SIGHUP to process
sleep 1

if ! grep -q "Received SIGHUP, reloading configuration..." "$LOG_FILE"; then
    fail "SIGHUP reload signal log was not found in logs!"
fi

if ! grep -q "Successfully reloaded server configuration" "$LOG_FILE"; then
    fail "Reload configuration success message was not found in logs!"
fi

echo "=== 6. Verifying Routing Logs ==="
echo "Checking proxy logs for correct Read/Write splitting..."

if ! grep -q "Routing command to Master" "$LOG_FILE"; then
    fail "Write command (SET) was not routed to Master in logs"
fi

if ! grep -q "Routing command to Replica" "$LOG_FILE"; then
    fail "Read command (GET) was not routed to Replica in logs"
fi

if ! grep -q "Command: \[SET mytestkey integration_test_passed\], Status: SUCCESS" "$LOG_FILE"; then
    fail "Default log format for SET not found in logs!"
fi

if ! grep -q "Command: \[GET mytestkey\], Status: SUCCESS" "$LOG_FILE"; then
    fail "Default log format for GET not found in logs!"
fi

echo "=== 7. Stopping Redis Proxy via CLI ==="
./redis-proxy stop

if [ -f redis-proxy.pid ]; then
    fail "PID file was not deleted after stopping the proxy!"
fi

if kill -0 "$PROXY_PID" >/dev/null 2>&1; then
    fail "Proxy process is still running after stop command!"
fi

echo "=== ALL INTEGRATION TESTS PASSED ==="
