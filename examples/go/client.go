package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	// 1. Configure the Redis client
	// Point to the proxy host and port (default is localhost:16379)
	// We omit the password since the proxy handles authentication to the backends.
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:16379",
		Password:     "", // Proxy handles upstream authentication
		DB:           0,  // Default DB
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	defer rdb.Close()

	fmt.Println("Connecting to Redis Proxy at 127.0.0.1:16379...")

	// Test connection with PING (routes to replicas/master depending on health/load balancer)
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis proxy: %v\nMake sure the Redis proxy is running.", err)
	}
	fmt.Printf("PING response: %s\n\n", pong)

	// 2. Write command (routes to Master)
	key := "sample_key:go"
	val := fmt.Sprintf("hello-from-go-at-%d", time.Now().Unix())
	fmt.Printf("Writing to proxy: SET %s -> %s (Routed to Master)\n", key, val)

	setErr := rdb.Set(ctx, key, val, 0).Err()
	if setErr != nil {
		log.Fatalf("SET failed: %v", setErr)
	}
	fmt.Println("SET response: OK\n")

	// Wait a moment for replication sync
	time.Sleep(500 * time.Millisecond)

	// 3. Read command (routes to Replica using the load-balancing strategy)
	fmt.Printf("Reading from proxy: GET %s (Routed to Replicas)\n", key)
	for i := 1; i <= 3; i++ {
		retrievedVal, err := rdb.Get(ctx, key).Result()
		if err != nil {
			log.Printf("  Attempt %d failed: %v", i, err)
			continue
		}
		fmt.Printf("  Attempt %d retrieved: %s\n", i, retrievedVal)
	}
}
