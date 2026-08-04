const { createClient } = require('redis');

async function main() {
  // 1. Configure and create the Redis client
  // The url points to the proxy running on 127.0.0.1:16379.
  // We do not specify credentials because the proxy authenticates to the backends.
  const client = createClient({
    url: 'redis://127.0.0.1:16379',
    socket: {
      connectTimeout: 2000
    }
  });

  client.on('error', (err) => {
    console.error('Redis Client Error:', err.message);
  });

  try {
    console.log('Connecting to Redis Proxy at 127.0.0.1:16379...');
    await client.connect();

    // Test connection
    console.log('Sending PING (routes to one of the replicas or master)...');
    const pingResponse = await client.ping();
    console.log(`PING response: ${pingResponse}\n`);

    // 2. Write command (routes to Master)
    const key = 'sample_key:javascript';
    const val = `hello-from-javascript-at-${Math.floor(Date.now() / 1000)}`;
    console.log(`Writing to proxy: SET ${key} -> ${val} (Routed to Master)`);
    const setResponse = await client.set(key, val);
    console.log(`SET response: ${setResponse}\n`);

    // Wait a brief moment to ensure replication sync to replicas
    await new Promise((resolve) => setTimeout(resolve, 500));

    // 3. Read command (routes to Replica using the load-balancing strategy)
    console.log(`Reading from proxy: GET ${key} (Routed to Replicas)`);
    for (let i = 1; i <= 3; i++) {
      const retrievedVal = await client.get(key);
      console.log(`  Attempt ${i} retrieved: ${retrievedVal}`);
    }

  } catch (err) {
    console.error('An error occurred during execution:', err);
  } finally {
    // Gracefully disconnect from proxy
    await client.disconnect();
  }
}

main();
