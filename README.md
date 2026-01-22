# Go Localization Backend

A Go Fiber application for localization services with comprehensive load testing tools.

> 🚀 **[Quick Start Guide](QUICK_START.md)** | 📖 **[Load Testing Guide](LOAD_TESTING.md)** | 🔥 **[Connection Hogging Test](CONNECTION_HOGGING_TEST.md)** | 💥 **[EXTREME Hog Test](EXTREME_HOG_TEST.md)**

## Quick Start with Docker

### Start the server
```bash
make up
# or
make docker-up
```

### Stop the server
```bash
make down
# or
make docker-down
```

### View logs
```bash
make docker-logs
```

### Restart the server
```bash
make docker-restart
```

## API Endpoints

- **GET** `/health` - Health check endpoint
- **POST** `/experiment` - A/B testing endpoint that returns a deterministic payload based on user ID

## Testing the Endpoints

### Health Check
```bash
curl http://localhost:3000/health
```

### Experiment Endpoint
```bash
curl -X POST http://localhost:3000/experiment \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-123"}'
```

**Request Body:**
```json
{
  "userId": "user-123"
}
```

**Response:**
```json
{
  "experimentId": "exp-localization-v1",
  "selectedPayloadName": "small_payload.json",
  "payload": "{ ... payload content ... }"
}
```

## A/B Testing Implementation

The `/experiment` endpoint implements deterministic A/B testing:

- **Payload Loading**: All JSON files from the `payloads/` directory are loaded at startup, sorted alphabetically for consistent ordering
- **Deterministic Assignment**: Uses FNV-1a hash of the `userId` to assign users to payloads. The same user always receives the same payload
- **Even Distribution**: Users are evenly distributed across all available payloads using `hash % numPayloads`

This ensures that each user consistently receives the same localization payload across multiple requests, which is essential for A/B testing integrity.

## Slow Client Protection

### The Problem

When serving large payloads (~1MB) over the internet, slow clients pose a significant risk:

1. **Connection Hogging**: A client on a slow network (e.g., 10KB/s) takes ~100 seconds to download 1MB. During this time, the server goroutine is blocked waiting to write to that client's TCP socket.

2. **Resource Exhaustion**: If enough slow clients connect simultaneously, all server resources (goroutines, connections, memory) get tied up, and fast clients experience degraded performance or timeouts.

3. **Cascading Failures**: Under load, new requests queue up waiting for connections, latencies spike, and the server can become unresponsive.

### Server-Side Protections

The server implements several protections in the Fiber configuration:

```go
ReadTimeout:  5 * time.Second   // Max time to read request
WriteTimeout: 10 * time.Second  // Max time to write response (KEY protection)
IdleTimeout:  30 * time.Second  // Max idle time on keep-alive connections
Concurrency:  10000             // Max concurrent connections
BodyLimit:    1 * 1024 * 1024   // Max request body size (1MB)
```

**WriteTimeout is the critical setting** - if a client can't receive the full response within 10 seconds, the connection is closed. This prevents slow clients from indefinitely holding server resources.

### Production Recommendation: Reverse Proxy Buffering

While server-side timeouts help, the **recommended production solution** is to put a reverse proxy (nginx, HAProxy, or a cloud load balancer) in front of the application:

```
[Slow Client] <--slow--> [nginx] <--fast--> [Go Backend]
```

**How it works:**
1. The Go backend writes the full 1MB response to nginx in milliseconds (fast local connection)
2. nginx buffers the response in memory/disk
3. nginx handles the slow delivery to the client separately
4. The Go backend is immediately free to handle the next request

**nginx configuration example:**
```nginx
location /experiment {
    proxy_pass http://backend:3000;
    proxy_buffering on;
    proxy_buffer_size 128k;
    proxy_buffers 4 256k;
    proxy_busy_buffers_size 256k;

    # nginx handles slow clients, not your app
    proxy_read_timeout 5s;      # Backend should respond quickly
    send_timeout 120s;          # Allow slow client downloads
}
```

This architecture ensures:
- **Fast clients stay fast**: Backend latency remains low regardless of slow clients
- **Slow clients still work**: They just get served by nginx, not your app server
- **Better resource utilization**: Go handles requests/sec, nginx handles bytes/sec

### Testing Slow Client Behavior

Use the allocation test to verify consistent behavior under load:
```bash
make load-test-allocation
```

Use the saturation test to observe slow client impact:
```bash
make load-test-saturation
```

## Load Testing

The project includes comprehensive load testing tools to test server resilience under concurrent fast and slow requests.

> 📖 For detailed load testing documentation, see [LOAD_TESTING.md](LOAD_TESTING.md)

### Quick Load Tests

```bash
# Basic load test (10 fast clients, 5 slow clients, 30s)
make load-test

# Light load test (5 fast, 2 slow, 20s)
make load-test-light

# Heavy load test (50 fast, 20 slow, 60s)
make load-test-heavy

# Stress test (100 fast, 50 slow, 120s)
make load-test-stress

# Connection hogging test (demonstrates slow clients hogging connections)
make load-test-hog

# EXTREME hogging test (only 1 concurrent connection!)
make load-test-hog-extreme
```

### Custom Load Test

Use the Go load testing script with custom parameters:

```bash
go run cmd/loadtest/main.go \
  -url http://localhost:3000 \
  -fast 20 \
  -slow 10 \
  -requests 100 \
  -slow-speed 1024 \
  -duration 60s
```

**Parameters:**
- `-url`: Server URL (default: http://localhost:3000)
- `-fast`: Number of fast clients (default: 10)
- `-slow`: Number of slow clients (default: 5)
- `-requests`: Requests per client (default: 100)
- `-slow-speed`: Slow client download speed in bytes/sec (default: 1024) - simulates slow network
- `-duration`: Test duration (default: 30s)
- `-hog-test`: Run connection hogging test (automatically adjusts clients and speed)

### Simple Bash Load Test

For a simpler shell-based test:

```bash
# Basic usage (10 fast, 5 slow, 30s)
./simple_load_test.sh

# Custom parameters: URL FAST_CLIENTS SLOW_CLIENTS DURATION
./simple_load_test.sh http://localhost:3000 20 10 60
```

### Understanding the Results

The load tests measure:
- **Total Requests**: Total number of requests sent
- **Success Rate**: Percentage of successful responses
- **Latency Statistics**: Min, Max, Average, and Percentiles (p50, p90, p99)
- **Throughput**: Requests per second
- **Performance Assessment**: Automatic evaluation of results

**Key Metrics Explained:**
- **p50 (median)**: 50% of requests complete faster than this
- **p90**: 90% of requests complete faster than this
- **p99**: 99% of requests complete faster than this (critical for tail latency)
- **Fast Client Latency**: Separate tracking for fast clients - watch this to see if slow clients impact fast ones
- **Slow Client Latency**: Includes slow download time - expected to be higher

**Good Performance Indicators:**
- p50 < 50ms (Excellent), < 100ms (Good)
- p99 < 200ms (Excellent), < 500ms (Good)
- Success rate > 99%
- **Fast clients maintain low latency even with slow network clients** (this is key!)

### Special Test: Connection Hogging

The connection hogging test (`make load-test-hog`) demonstrates how slow clients can impact fast clients:

```bash
make load-test-hog
```

This test:
- Floods the server with many slow clients (50+)
- Uses very slow download speeds (128 bytes/sec)
- Tracks fast client latency separately
- Shows if slow clients are blocking server resources

**What to watch**: If fast client p99 latency increases significantly, your server is experiencing connection hogging.

### Extreme Test: 1 Connection Only

For the ultimate demonstration of hogging:

```bash
make load-test-hog-extreme
```

This test:
- Limits server to **ONLY 1 concurrent connection**
- Makes the problem impossible to miss
- Shows throughput collapse (50+ req/s → ~1 req/s)
- Demonstrates 200-700x latency increase
- Perfect for demonstrating the problem to stakeholders

See [EXTREME_HOG_TEST.md](EXTREME_HOG_TEST.md) for detailed documentation.

## Development

### Run locally (without Docker)
```bash
make run
```

### Build the application
```bash
make build
```

### Install dependencies
```bash
make deps
```

### Format code
```bash
make fmt
```

### See all available commands
```bash
make help
```

## Requirements

- Docker and Docker Compose (for containerized deployment)
- Go 1.23.1+ (for local development)

## Project Structure

```
.
├── main.go                      # Main application file
├── go.mod                       # Go module file
├── go.sum                       # Go dependencies checksum
├── Makefile                     # Build and run commands
├── Dockerfile                   # Docker image definition
├── docker-compose.yml           # Docker Compose configuration
├── simple_load_test.sh          # Simple load testing script (Bash)
├── demo_test.sh                 # Quick demo script
├── cmd/
│   └── loadtest/
│       └── main.go              # Advanced load testing tool (Go)
└── payloads/                    # JSON payload examples
```

## Complete Workflow Example

```bash
# 1. Start the server
make up

# 2. Test basic functionality
curl http://localhost:3000/health

# 3. Run load test
make load-test

# 4. View server logs
make docker-logs

# 5. Stop the server
make down
```

