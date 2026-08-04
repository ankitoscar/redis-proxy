#!/usr/bin/env python3
import time
import redis

def main():
    # 1. Configure the connection details
    # The proxy runs on 127.0.0.1:16379. We do not need a password,
    # because the proxy handles upstream Redis authentication transparently.
    redis_host = "127.0.0.1"
    redis_port = 16379

    print(f"Connecting to Redis Proxy at {redis_host}:{redis_port}...")
    
    # Initialize the Redis client
    r = redis.Redis(
        host=redis_host,
        port=redis_port,
        decode_responses=True, # Automatically decode bytes to strings
        socket_timeout=2.0
    )

    try:
        # Test connection
        print("Sending PING (routes to one of the replicas or master)...")
        ping_response = r.ping()
        print(f"PING response: {ping_response}\n")

        # 2. Write command (routes to Master)
        key = "sample_key:python"
        val = f"hello-from-python-at-{int(time.time())}"
        print(f"Writing to proxy: SET {key} -> {val} (Routed to Master)")
        set_ok = r.set(key, val)
        print(f"SET response: {set_ok}\n")

        # Wait a brief moment to ensure replication sync to replicas
        time.sleep(0.5)

        # 3. Read command (routes to Replica using the load-balancing strategy)
        print(f"Reading from proxy: GET {key} (Routed to Replicas)")
        for i in range(3):
            retrieved_val = r.get(key)
            print(f"  Attempt {i+1} retrieved: {retrieved_val}")
            
    except redis.ConnectionError as e:
        print(f"Could not connect to Redis proxy: {e}")
        print("Make sure the Redis proxy is running on port 16379.")
    except Exception as e:
        print(f"An error occurred: {e}")

if __name__ == "__main__":
    main()
