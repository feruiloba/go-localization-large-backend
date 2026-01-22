# Implementation Summary

This document describes the implementation of three tasks for the Go Localization Backend.

---

## 1. Basic A/B Testing Implementation

### Objective

Implement deterministic A/B testing where each user consistently receives the same payload from the `/payloads` directory, with even distribution across all available payloads.

### Implementation

**File:** `main.go`

#### Payload Loading (lines 26-62)

At server startup, all JSON payload files are loaded into memory:

```go
type Payload struct {
    Name    string
    Content string
}

var payloads []Payload
```

The `init()` function:
1. Reads all `.json` files from the `payloads/` directory
2. Sorts filenames alphabetically to ensure deterministic ordering across restarts
3. Loads each file's content into the `payloads` slice
4. Logs the number of payloads loaded (currently 6 files, ranging from 262 bytes to 1.1MB)

#### Deterministic Assignment (lines 122-127)

Users are assigned to payloads using FNV-1a hashing:

```go
func getPayloadForUser(userID string) Payload {
    h := fnv.New32a()
    h.Write([]byte(userID))
    index := int(h.Sum32()) % len(payloads)
    return payloads[index]
}
```

**Why FNV-1a?**
- Fast, non-cryptographic hash suitable for bucketing
- Deterministic: same input always produces same output
- Good distribution: users are evenly spread across buckets

**Why `hash % numPayloads`?**
- Simple modulo operation assigns each hash to exactly one payload
- With 6 payloads, each gets approximately 16.67% of users
- Adding/removing payloads would reassign users (acceptable for this use case)

#### Request Handler (lines 95-119)

The `/experiment` endpoint:
1. Parses the JSON request body into `model.Request` struct
2. Validates that `userId` is present
3. Calls `getPayloadForUser()` to get the deterministic assignment
4. Returns a `model.Response` with experiment ID, payload name, and full payload content

```go
response := model.Response{
    ExperimentID:        "exp-localization-v1",
    SelectedPayloadName: payload.Name,
    Payload:             payload.Content,
}
```

### Verification

Same user always receives the same payload:
```bash
# All three return the same payload
curl -X POST localhost:3000/experiment -d '{"userId":"user-123"}'
curl -X POST localhost:3000/experiment -d '{"userId":"user-123"}'
curl -X POST localhost:3000/experiment -d '{"userId":"user-123"}'
```

---

## 2. A/B Allocation Load Test

### Objective

Create a load test that verifies:
1. Each user consistently receives the same payload across multiple requests
2. Users are distributed across all available payloads
3. Output results to a file

### Implementation

**File:** `cmd/allocationtest/main.go`

#### Test Design

The allocation test:
1. Generates N unique user IDs (UUIDs)
2. Makes M requests per user concurrently
3. Tracks which payload each user receives on each request
4. Verifies consistency (each user should only receive one payload)
5. Reports distribution across payloads

#### Key Components

**Work Distribution:**
```go
type work struct {
    userID string
}
workChan := make(chan work, len(userIDs)*requestsPerUser)
```

A worker pool processes requests concurrently, with configurable concurrency level.

**Consistency Checking:**
```go
userPayloads := make(map[string]map[string]int) // userID -> payloadName -> count
```

For each user, we track how many times they received each payload. A consistent user should have exactly one entry.

**Results Analysis:**
```go
consistent := len(payloads) == 1  // User received only one payload type
```

#### Output

Results are written to `allocation_test_results.md` containing:
- Test configuration (users, requests, duration)
- Request statistics (total, successful, failed)
- Consistency rate (should be 100%)
- Payload distribution table
- Sample user allocations

#### Running the Test

```bash
make load-test-allocation
# Or directly:
go run cmd/allocationtest/main.go -users 100 -requests 5 -concurrency 10
```

#### Sample Results

```
Allocation Consistency:
  Total Users: 100
  Consistent Users: 100
  Inconsistent Users: 0
  Consistency Rate: 100.00%

Payload Distribution:
  localization_dummy_3.json: 17 users (17.0%)
  localization_dummy_4.json: 17 users (17.0%)
  localization_example.json: 15 users (15.0%)
  localization_example_2.json: 10 users (10.0%)
  nested_large.json: 21 users (21.0%)
  small_payload.json: 20 users (20.0%)
```

The distribution is not perfectly even (10-21%) due to the nature of hash distribution with a small number of buckets, but it's reasonably balanced.

---

## 3. Production Hardening: Slow Client Protection

### The Problem

When serving large payloads (~1MB) over the internet:

1. **Slow clients hog connections**: A client on a 10KB/s connection takes ~100 seconds to download 1MB. The server goroutine remains blocked, waiting to write to that client's TCP socket.

2. **Resource exhaustion**: With enough slow clients, all server goroutines become occupied, and fast clients experience queuing, increased latency, or timeouts.

3. **Cascading failure**: Under load, the server becomes unresponsive to all clients.

### Solution: Server-Side Timeouts

**File:** `main.go` (lines 66-93)

Added Fiber configuration with protective timeouts:

```go
app := fiber.New(fiber.Config{
    // Request read timeout - protects against slow request senders
    ReadTimeout: 5 * time.Second,

    // Response write timeout - KEY PROTECTION against slow clients
    // If client can't receive 1MB in 10s, connection is closed
    WriteTimeout: 10 * time.Second,

    // Idle connection timeout - frees connections from idle clients
    IdleTimeout: 30 * time.Second,

    // Hard cap on concurrent connections
    Concurrency: 10000,

    // Prevent memory exhaustion from large request bodies
    BodyLimit: 1 * 1024 * 1024,
})
```

#### Why These Values?

| Setting | Value | Rationale |
|---------|-------|-----------|
| `ReadTimeout` | 5s | Requests are small JSON; 5s is generous |
| `WriteTimeout` | 10s | 1MB at 100KB/s = 10s; slower clients get disconnected |
| `IdleTimeout` | 30s | Standard keep-alive timeout |
| `Concurrency` | 10,000 | Reasonable limit; prevents runaway resource usage |
| `BodyLimit` | 1MB | Matches our largest expected request |

#### Trade-offs

**Pros:**
- Prevents resource exhaustion
- Fast clients remain fast
- Simple to implement

**Cons:**
- Very slow clients (< 100KB/s) get disconnected
- Users on poor connections may experience failures

### Recommended Production Architecture: Reverse Proxy Buffering

For production, add a reverse proxy (nginx/HAProxy) with response buffering. This is **standard practice** - almost no production service exposes application servers directly to the internet. The typical architecture is:

```
Internet → Load Balancer/Proxy → Application Server
```

Services like AWS ALB/ELB, Cloudflare, and Kubernetes Ingress all implement this pattern.

#### The Problem Visualized

**Without nginx (direct connection):**
```
Client (slow 3G) <----100 seconds----> Go Backend
                                       ↑
                                       Goroutine BLOCKED
                                       waiting to write 1MB
```

The Go server must wait for the slow client to receive all the data. During those 100 seconds, that goroutine is blocked and can't do anything else.

**With nginx buffering:**
```
Client (slow 3G) <--100 sec--> [nginx buffer] <--50ms--> Go Backend
                                     ↑                        ↑
                               nginx handles              Goroutine FREE
                               slow delivery              after 50ms
```

#### How Buffering Works

1. **Go backend responds fast**: Your app writes the 1MB response to nginx over a local network (or unix socket). This takes milliseconds because it's machine-to-machine, not over the internet.

2. **nginx stores the response**: nginx puts the full response in memory (or disk for very large responses).

3. **nginx drip-feeds the client**: nginx sends data to the slow client at whatever speed their connection allows. This might take minutes, but nginx is designed for this.

4. **Go is already done**: Your Go backend finished its work in 50ms and moved on to serve other requests.

#### Why nginx Excels at Slow Connections

The key difference is in how nginx and Go handle concurrent connections:

| | Go Backend | nginx |
|--|-----------|-------|
| **Architecture** | Goroutine per request | Event loop |
| **Memory per connection** | ~2-8KB (goroutine stack) | ~256 bytes |
| **Optimized for** | Application logic, fast responses | I/O, connection handling |

**nginx uses an event-driven architecture**, not threads or goroutines. It runs a single-threaded event loop (per worker process) that uses OS primitives like `epoll` (Linux) or `kqueue` (BSD/macOS) to monitor thousands of connections simultaneously.

When a slow client can only receive 1KB of data, nginx:
1. Writes that 1KB to the socket
2. Registers interest in "socket ready for more writing"
3. Immediately moves on to handle other connections
4. Gets notified later when the socket is ready for more data

This non-blocking I/O model means nginx can handle **10,000+ slow connections with minimal CPU and memory**, because it's not dedicating a thread or goroutine to each one. It just loops through ready sockets, does a tiny bit of work on each, and continues.

In contrast, Go's model (goroutine per request) works great for application logic but is wasteful when goroutines are just waiting on slow I/O. Each blocked goroutine still consumes stack memory and scheduler overhead.

#### nginx Configuration

```nginx
location /experiment {
    proxy_pass http://backend:3000;
    proxy_buffering on;
    proxy_buffer_size 128k;
    proxy_buffers 4 256k;
    proxy_busy_buffers_size 256k;
    proxy_read_timeout 5s;   # Backend should respond quickly
    send_timeout 120s;       # Allow slow client downloads
}
```

#### The Key Insight

**Your Go server should be optimized for requests per second (application logic). nginx is optimized for bytes per second (I/O). Let each do what it's best at.**

This separation of concerns is why the proxy pattern is universal in production:
- The proxy handles: SSL termination, slow clients, connection pooling, rate limiting, compression
- The application handles: business logic, database queries, response generation

### Testing Slow Client Impact

The existing load test tools can demonstrate the slow client problem:

```bash
# Normal test - all fast clients
make load-test-normal

# Saturation test - introduces slow clients to show impact
make load-test-saturation
```

The saturation test uses a `SlowReader` that throttles download speed to simulate slow network conditions, allowing observation of how slow clients affect fast client latency.

---

## Summary of Changes

| File | Changes |
|------|---------|
| `main.go` | A/B testing logic, timeout configuration |
| `cmd/allocationtest/main.go` | New allocation verification tool |
| `Makefile` | Added `load-test-allocation` target |
| `README.md` | Documentation for A/B testing and slow client protection |
| `allocation_test_results.md` | Sample test output |
| `CLAUDE.md` | Development guidance for Claude Code |
