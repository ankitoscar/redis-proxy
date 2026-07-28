#!/bin/bash
set -e

# File to store proxy logs during the integration test
LOG_FILE="redis-proxy.log"
rm -f "$LOG_FILE"
rm -f redis-proxy.pid

echo "=== 1. Starting Redis Docker Containers ==="
docker compose -f tests/docker-compose.yml up -d

# Cleanup function to run on script exit
cleanup() {
    echo "=== Cleaning up ==="
    if [ -f redis-proxy.pid ]; then
        echo "Stopping Redis Proxy using CLI stop..."
        ./redis-proxy stop || true
    fi
    echo "Stopping Docker Containers..."
    docker compose -f tests/docker-compose.yml down
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
    docker compose -f tests/docker-compose.yml logs || true
    echo ""
    echo "=== 3. Master Replication Info ==="
    redis-cli -p 6379 -a mysecretpassword info replication || true
    echo ""
    echo "=== 4. Replica 1 Replication Info ==="
    redis-cli -p 6380 -a mysecretpassword info replication || true
    echo ""
    echo "=== 5. Replica 2 Replication Info ==="
    redis-cli -p 6381 -a mysecretpassword info replication || true
    exit 1
}

# Wait for master (6379) and replicas (6380, 6381) to be fully ready
echo "Waiting for Redis instances to accept connections..."
until redis-cli -p 6379 -a mysecretpassword PING >/dev/null 2>&1; do
    echo -n "."
    sleep 1
done
until redis-cli -p 6380 -a mysecretpassword PING >/dev/null 2>&1; do
    echo -n "."
    sleep 1
done
until redis-cli -p 6381 -a mysecretpassword PING >/dev/null 2>&1; do
    echo -n "."
    sleep 1
done
echo " Redis instances are responding to PING."

# Wait for replication handshake to complete
echo "Waiting for Replicas to establish link with Master..."
until redis-cli -p 6380 -a mysecretpassword info replication | grep -q "master_link_status:up"; do
    echo -n "."
    sleep 1
done
until redis-cli -p 6381 -a mysecretpassword info replication | grep -q "master_link_status:up"; do
    echo -n "."
    sleep 1
done
echo " Replica links are UP and synchronized!"

echo "=== 2. Building the Redis Proxy ==="
go build -o redis-proxy .

echo "=== 3. Starting the Redis Proxy CLI Daemon (Random Load Balancing) ==="
# Ensure conf starts with random
sed -i 's/load_balance = .*/load_balance = random/g' redis-proxy.conf

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

echo "=== 4. Running Test Commands (Random) ==="
echo "Sending SET command (write) to proxy..."
SET_RESP=$(redis-cli -h 127.0.0.1 -p 16379 SET mytestkey "integration_test_passed" 2>&1) || fail "SET command failed to run"
echo "SET response: $SET_RESP"

if [ "$SET_RESP" != "OK" ]; then
    fail "SET command returned '$SET_RESP' instead of 'OK'"
fi

sleep 1

echo "Sending GET commands (read) to proxy..."
GET_RESP1=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET command 1 failed to run"
GET_RESP2=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET command 2 failed to run"
echo "GET responses: $GET_RESP1, $GET_RESP2"

if [ "$GET_RESP1" != "integration_test_passed" ] || [ "$GET_RESP2" != "integration_test_passed" ]; then
    fail "One of the GET commands did not return 'integration_test_passed'"
fi

echo "Updating key (SET) to a new value..."
SET_RESP_UPDATE=$(redis-cli -h 127.0.0.1 -p 16379 SET mytestkey "updated_value" 2>&1) || fail "SET update command failed to run"
echo "SET update response: $SET_RESP_UPDATE"

if [ "$SET_RESP_UPDATE" != "OK" ]; then
    fail "SET update command returned '$SET_RESP_UPDATE' instead of 'OK'"
fi

sleep 1

echo "Sending GET command to retrieve the updated key..."
GET_RESP_UPDATE=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET updated key failed to run"
echo "GET updated response: $GET_RESP_UPDATE"

if [ "$GET_RESP_UPDATE" != "updated_value" ]; then
    fail "GET updated command returned '$GET_RESP_UPDATE' instead of 'updated_value'"
fi

echo "=== 5. Switching to Round-Robin & CLI Reload ==="
# Update configuration to round-robin
sed -i 's/load_balance = .*/load_balance = round-robin/g' redis-proxy.conf

# Reload the configuration
./redis-proxy reload

# Wait a moment for SIGHUP to process
sleep 1

if ! grep -q "Received SIGHUP, reloading configuration..." "$LOG_FILE"; then
    fail "SIGHUP reload signal log was not found in logs!"
fi

if ! grep -q "strategy: round-robin" "$LOG_FILE"; then
    fail "Reload configuration strategy: round-robin message was not found in logs!"
fi

echo "=== 6. Running Test Commands (Round-Robin) ==="
# Send 3 GET commands to verify round-robin behavior
GET_RESP3=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET command 3 failed to run"
GET_RESP4=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET command 4 failed to run"
GET_RESP5=$(redis-cli -h 127.0.0.1 -p 16379 GET mytestkey 2>&1) || fail "GET command 5 failed to run"
echo "GET responses: $GET_RESP3, $GET_RESP4, $GET_RESP5"

if [ "$GET_RESP3" != "updated_value" ] || [ "$GET_RESP4" != "updated_value" ] || [ "$GET_RESP5" != "updated_value" ]; then
    fail "One of the GET commands in round-robin did not return 'updated_value'"
fi

echo "=== 7. Verifying Routing Logs ==="
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

echo "=== 8. Stopping Redis Proxy via CLI ==="
./redis-proxy stop

if [ -f redis-proxy.pid ]; then
    fail "PID file was not deleted after stopping the proxy!"
fi

if kill -0 "$PROXY_PID" >/dev/null 2>&1; then
    fail "Proxy process is still running after stop command!"
fi

echo "=== ALL INTEGRATION TESTS PASSED ==="
