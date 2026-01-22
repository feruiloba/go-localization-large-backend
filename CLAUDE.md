# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build
make build              # Build to bin/main
make docker-build       # Build Docker image

# Run
make run                # Run locally on port 3000
make run-limited        # Run with MAX_CONNS=20
make dev                # Run with hot reload (requires air)
make up                 # Start Docker containers

# Test
make test               # Run Go tests
make load-test-normal   # Load test with fast clients only
make load-test-saturation  # Load test with slow+fast clients (connection hogging)

# Format and Lint
make fmt                # Format with gofmt
make lint               # Run golangci-lint

# Cleanup
make clean              # Remove build artifacts
make down               # Stop Docker containers
```

## Architecture Overview

This is a Go backend using Fiber v2 designed as a **performance testing harness** for localization services. The primary purpose is measuring server behavior under mixed client conditions (fast vs slow clients).

### Entry Points

- **main.go** - Fiber web server on port 3000 with two endpoints:
  - `GET /health` - Health check
  - `POST /experiment` - Returns pre-loaded 1MB JSON payload

- **cmd/loadtest/main.go** - Load testing CLI tool that simulates fast and slow clients to demonstrate connection hogging behavior

### Key Design Patterns

- **Payload Preloading**: 1MB JSON loaded into memory at `init()` for fast serving
- **SlowReader**: Custom reader type in load test tool that simulates network throttling with jitter
- **Atomic Operations**: Thread-safe counters for concurrent load testing statistics
- **Latency Percentiles**: Load test tracks p50, p90, p99 latencies separately for fast/slow clients

### Project Structure

- `main.go` - Server entry point
- `pkg/model/` - Request/Response structs
- `cmd/loadtest/` - Load testing tool
- `payloads/` - Test JSON payloads (262B to 1.1MB)
