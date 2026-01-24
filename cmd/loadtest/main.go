package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type TestConfig struct {
	ServerURL         string
	FastClients       int
	SlowClients       int
	RequestsPerClient int
	SlowDownloadSpeed int // bytes per second for slow clients
	TestDuration      time.Duration
	ConnectionHogTest bool // Special mode to demonstrate connection hogging
}

type Stats struct {
	totalRequests   atomic.Int64
	successRequests atomic.Int64
	failedRequests  atomic.Int64
	fastRequests    atomic.Int64
	slowRequests    atomic.Int64
	latenciesMutex  sync.Mutex
	fastLatencies   []int64 // fast client latencies in milliseconds
	slowLatencies   []int64 // slow client latencies in milliseconds
}

// SlowReader wraps an io.Reader to simulate slow network download speeds with random delays
type SlowReader struct {
	reader      io.Reader
	bytesPerSec int
	lastRead    time.Time
	rng         *rand.Rand
}

func NewSlowReader(reader io.Reader, bytesPerSec int) *SlowReader {
	return &SlowReader{
		reader:      reader,
		bytesPerSec: bytesPerSec,
		lastRead:    time.Now(),
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (sr *SlowReader) Read(p []byte) (n int, err error) {
	// Calculate how long we should wait based on the bytes per second rate
	chunkSize := sr.bytesPerSec / 10 // Read in 100ms chunks
	if chunkSize == 0 {
		chunkSize = 1
	}
	if chunkSize > len(p) {
		chunkSize = len(p)
	}

	// Read a chunk
	n, err = sr.reader.Read(p[:chunkSize])

	if n > 0 {
		// Calculate base delay to simulate slow download
		expectedDuration := time.Duration(float64(n) / float64(sr.bytesPerSec) * float64(time.Second))
		elapsed := time.Since(sr.lastRead)

		if expectedDuration > elapsed {
			baseDelay := expectedDuration - elapsed

			// Add random jitter (0-50% additional delay) to simulate realistic network variance
			jitter := time.Duration(float64(baseDelay) * sr.rng.Float64() * 0.5)
			totalDelay := baseDelay + jitter

			time.Sleep(totalDelay)
		}

		// Occasionally add a random stall (simulates network hiccups)
		if sr.rng.Float64() < 0.1 { // 10% chance of stall
			stallDuration := time.Duration(sr.rng.Intn(100)) * time.Millisecond
			time.Sleep(stallDuration)
		}

		sr.lastRead = time.Now()
	}

	return n, err
}

func main() {
	// Command line flags
	serverURL := flag.String("url", "http://localhost:3000", "Server URL")
	fastClients := flag.Int("fast", 10, "Number of fast clients")
	slowClients := flag.Int("slow", 5, "Number of slow clients")
	requests := flag.Int("requests", 100, "Requests per client")
	slowSpeed := flag.Int("slow-speed", 1024, "Slow client download speed in bytes/sec (simulates slow network)")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	hogTest := flag.Bool("hog-test", false, "Run connection hogging test (many slow clients, measure fast client impact)")
	mode := flag.String("mode", "normal", "Test mode: 'normal' (all fast) or 'saturation' (mix of slow/fast)")
	flag.Parse()

	// Apply mode presets
	if *mode == "saturation" {
		*hogTest = true
	}

	config := TestConfig{
		ServerURL:         *serverURL,
		FastClients:       *fastClients,
		SlowClients:       *slowClients,
		RequestsPerClient: *requests,
		SlowDownloadSpeed: *slowSpeed,
		TestDuration:      *duration,
		ConnectionHogTest: *hogTest,
	}

	// Adjust settings for saturation/hogging test
	if config.ConnectionHogTest {
		fmt.Println("🔥 Running Saturation/Hogging Test")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("This test demonstrates how slow clients can hog server connections")
		fmt.Println("and degrade performance for fast clients.")
		fmt.Println()

		// Override with settings designed to expose the problem
		if *slowClients == 5 { // Only override if default
			config.SlowClients = 50 // Many slow clients
		}
		if *fastClients == 10 { // Only override if default
			config.FastClients = 10 // Moderate fast clients
		}
		if *slowSpeed == 1024 { // Only override if default
			// Calculate speed to achieve approx 2 second download time for 1MB payload
			// 1MB = 1024 * 1024 bytes = 1,048,576 bytes
			// Speed = TotalBytes / Duration = 1,048,576 / 2 = ~524,288 bytes/sec
			config.SlowDownloadSpeed = 500 * 1024 // ~500KB/s
		}
	} else {
		// Normal mode defaults
		fmt.Println("🚀 Running Normal Load Test")
		config.SlowClients = 0 // No slow clients in normal mode by default
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Server URL: %s\n", config.ServerURL)
	fmt.Printf("Fast Clients: %d\n", config.FastClients)
	fmt.Printf("Slow Clients: %d (simulating %d bytes/sec network)\n", config.SlowClients, config.SlowDownloadSpeed)
	fmt.Printf("Requests per Client: %d\n", config.RequestsPerClient)
	fmt.Printf("Test Duration: %s\n", config.TestDuration)
	if config.ConnectionHogTest {
		fmt.Printf("Mode: Connection Hogging Test\n")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Check server health before starting
	if !checkHealth(config.ServerURL) {
		fmt.Println("❌ Server health check failed. Is the server running?")
		return
	}

	stats := &Stats{
		fastLatencies: make([]int64, 0, 10000),
		slowLatencies: make([]int64, 0, 10000),
	}

	// Start monitoring
	stopMonitor := make(chan bool)
	go monitorProgress(stats, stopMonitor)

	// Run the load test
	startTime := time.Now()
	runLoadTest(config, stats)
	endTime := time.Now()

	// Stop monitoring
	stopMonitor <- true
	time.Sleep(100 * time.Millisecond)

	// Print results
	printResults(stats, startTime, endTime, config)
}

func checkHealth(serverURL string) bool {
	resp, err := http.Get(serverURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func runLoadTest(config TestConfig, stats *Stats) {
	var wg sync.WaitGroup
	ctx := make(chan bool)

	// In saturation mode, start slow clients FIRST to hog connections
	// Then start fast clients to see if they are blocked
	if config.ConnectionHogTest {
		fmt.Println("   ... Pre-warming with slow clients to saturate connections ...")
		// Start slow clients
		for i := 0; i < config.SlowClients; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()
				runSlowClient(clientID, config, stats, ctx)
			}(i)
		}

		// Wait a bit to let slow clients establish connections
		time.Sleep(2 * time.Second)
		fmt.Println("   ... Starting fast clients now ...")

		// Start fast clients
		for i := 0; i < config.FastClients; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()
				runFastClient(clientID, config, stats, ctx)
			}(i)
		}
	} else {
		// Normal mode - start everything together
		// Start fast clients
		for i := 0; i < config.FastClients; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()
				runFastClient(clientID, config, stats, ctx)
			}(i)
		}

		// Start slow clients
		for i := 0; i < config.SlowClients; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()
				runSlowClient(clientID, config, stats, ctx)
			}(i)
		}
	}

	// Wait for test duration
	time.Sleep(config.TestDuration)
	close(ctx)

	// Wait for all clients to finish
	wg.Wait()
}

func runFastClient(_ int, config TestConfig, stats *Stats, ctx chan bool) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for i := 0; i < config.RequestsPerClient; i++ {
		select {
		case <-ctx:
			return
		default:
			makeFastRequest(client, config.ServerURL+"/experiment", stats)
			stats.fastRequests.Add(1)
			// Small delay between requests
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func runSlowClient(_ int, config TestConfig, stats *Stats, ctx chan bool) {
	client := &http.Client{
		Timeout: 60 * time.Second, // Longer timeout for slow downloads
	}

	for i := 0; i < config.RequestsPerClient; i++ {
		select {
		case <-ctx:
			return
		default:
			makeSlowRequest(client, config.ServerURL+"/experiment", config.SlowDownloadSpeed, stats)
			stats.slowRequests.Add(1)
			// Small delay between requests
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func makeFastRequest(client *http.Client, url string, stats *Stats) {
	stats.totalRequests.Add(1)

	// Generate a unique userId for each request
	userID := fmt.Sprintf("fast-user-%d", time.Now().UnixNano())
	payload := map[string]string{
		"userId": userID,
	}
	jsonData, _ := json.Marshal(payload)

	start := time.Now()
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))

	if err != nil {
		stats.failedRequests.Add(1)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// Read response body normally (fast)
		_, err = io.Copy(io.Discard, resp.Body)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			stats.successRequests.Add(1)
			stats.latenciesMutex.Lock()
			stats.fastLatencies = append(stats.fastLatencies, latency)
			stats.latenciesMutex.Unlock()
		} else {
			stats.failedRequests.Add(1)
		}
	} else {
		stats.failedRequests.Add(1)
	}
}

func makeSlowRequest(client *http.Client, url string, bytesPerSec int, stats *Stats) {
	stats.totalRequests.Add(1)

	// Generate a unique userId for each request
	userID := fmt.Sprintf("slow-user-%d", time.Now().UnixNano())
	payload := map[string]string{
		"userId": userID,
	}
	jsonData, _ := json.Marshal(payload)

	start := time.Now()
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))

	if err != nil {
		stats.failedRequests.Add(1)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// Simulate slow network by reading response body slowly with random delays
		slowReader := NewSlowReader(resp.Body, bytesPerSec)
		_, err = io.Copy(io.Discard, slowReader)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			stats.successRequests.Add(1)
			stats.latenciesMutex.Lock()
			stats.slowLatencies = append(stats.slowLatencies, latency)
			stats.latenciesMutex.Unlock()
		} else {
			stats.failedRequests.Add(1)
		}
	} else {
		stats.failedRequests.Add(1)
	}
}

func monitorProgress(stats *Stats, stop chan bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			total := stats.totalRequests.Load()
			success := stats.successRequests.Load()
			failed := stats.failedRequests.Load()
			fast := stats.fastRequests.Load()
			slow := stats.slowRequests.Load()

			fmt.Printf("\r📊 Progress: Total: %d | Success: %d | Failed: %d | Fast: %d | Slow: %d",
				total, success, failed, fast, slow)
		}
	}
}

func calculatePercentile(sortedLatencies []int64, percentile float64) int64 {
	if len(sortedLatencies) == 0 {
		return 0
	}
	index := int(float64(len(sortedLatencies)) * percentile)
	if index >= len(sortedLatencies) {
		index = len(sortedLatencies) - 1
	}
	return sortedLatencies[index]
}

func printResults(stats *Stats, startTime, endTime time.Time, config TestConfig) {
	totalRequests := stats.totalRequests.Load()
	successRequests := stats.successRequests.Load()
	failedRequests := stats.failedRequests.Load()
	fastRequests := stats.fastRequests.Load()
	slowRequests := stats.slowRequests.Load()

	duration := endTime.Sub(startTime)

	// Sort latencies for percentile calculation
	stats.latenciesMutex.Lock()
	fastLatencies := make([]int64, len(stats.fastLatencies))
	slowLatencies := make([]int64, len(stats.slowLatencies))
	copy(fastLatencies, stats.fastLatencies)
	copy(slowLatencies, stats.slowLatencies)
	stats.latenciesMutex.Unlock()

	sort.Slice(fastLatencies, func(i, j int) bool {
		return fastLatencies[i] < fastLatencies[j]
	})
	sort.Slice(slowLatencies, func(i, j int) bool {
		return slowLatencies[i] < slowLatencies[j]
	})

	// Combine all latencies for overall stats
	allLatencies := make([]int64, 0, len(fastLatencies)+len(slowLatencies))
	allLatencies = append(allLatencies, fastLatencies...)
	allLatencies = append(allLatencies, slowLatencies...)
	sort.Slice(allLatencies, func(i, j int) bool {
		return allLatencies[i] < allLatencies[j]
	})

	// Calculate overall statistics
	var minLatency, maxLatency, avgLatency, p50, p90, p99 int64
	if len(allLatencies) > 0 {
		minLatency = allLatencies[0]
		maxLatency = allLatencies[len(allLatencies)-1]

		var totalLatency int64
		for _, lat := range allLatencies {
			totalLatency += lat
		}
		avgLatency = totalLatency / int64(len(allLatencies))

		p50 = calculatePercentile(allLatencies, 0.50)
		p90 = calculatePercentile(allLatencies, 0.90)
		p99 = calculatePercentile(allLatencies, 0.99)
	}

	// Calculate fast client statistics
	var fastMin, fastMax, fastAvg, fastP50, fastP90, fastP99 int64
	if len(fastLatencies) > 0 {
		fastMin = fastLatencies[0]
		fastMax = fastLatencies[len(fastLatencies)-1]

		var totalFastLatency int64
		for _, lat := range fastLatencies {
			totalFastLatency += lat
		}
		fastAvg = totalFastLatency / int64(len(fastLatencies))

		fastP50 = calculatePercentile(fastLatencies, 0.50)
		fastP90 = calculatePercentile(fastLatencies, 0.90)
		fastP99 = calculatePercentile(fastLatencies, 0.99)
	}

	// Calculate slow client statistics
	var slowMin, slowMax, slowAvg, slowP50, slowP90, slowP99 int64
	if len(slowLatencies) > 0 {
		slowMin = slowLatencies[0]
		slowMax = slowLatencies[len(slowLatencies)-1]

		var totalSlowLatency int64
		for _, lat := range slowLatencies {
			totalSlowLatency += lat
		}
		slowAvg = totalSlowLatency / int64(len(slowLatencies))

		slowP50 = calculatePercentile(slowLatencies, 0.50)
		slowP90 = calculatePercentile(slowLatencies, 0.90)
		slowP99 = calculatePercentile(slowLatencies, 0.99)
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📈 Load Test Results")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Test Duration: %s\n", duration.Round(time.Millisecond))
	fmt.Println()

	fmt.Println("Request Statistics:")
	fmt.Printf("  Total Requests:   %d\n", totalRequests)
	fmt.Printf("  Successful:       %d (%.2f%%)\n", successRequests, float64(successRequests)/float64(totalRequests)*100)
	fmt.Printf("  Failed:           %d (%.2f%%)\n", failedRequests, float64(failedRequests)/float64(totalRequests)*100)
	fmt.Printf("  Fast Clients:     %d\n", fastRequests)
	fmt.Printf("  Slow Clients:     %d\n", slowRequests)
	fmt.Println()

	fmt.Println("Overall Latency Statistics:")
	fmt.Printf("  Minimum:          %d ms\n", minLatency)
	fmt.Printf("  Average:          %d ms\n", avgLatency)
	fmt.Printf("  Maximum:          %d ms\n", maxLatency)
	fmt.Println()

	fmt.Println("Overall Latency Percentiles:")
	fmt.Printf("  p50 (median):     %d ms\n", p50)
	fmt.Printf("  p90:              %d ms\n", p90)
	fmt.Printf("  p99:              %d ms\n", p99)
	fmt.Println()

	// Print detailed fast client stats
	if len(fastLatencies) > 0 {
		fmt.Println("Fast Client Latency (KEY METRIC):")
		fmt.Printf("  Minimum:          %d ms\n", fastMin)
		fmt.Printf("  Average:          %d ms\n", fastAvg)
		fmt.Printf("  Maximum:          %d ms\n", fastMax)
		fmt.Printf("  p50:              %d ms\n", fastP50)
		fmt.Printf("  p90:              %d ms\n", fastP90)
		fmt.Printf("  p99:              %d ms\n", fastP99)
		fmt.Println()
	}

	// Print detailed slow client stats
	if len(slowLatencies) > 0 {
		fmt.Println("Slow Client Latency (includes download time):")
		fmt.Printf("  Minimum:          %d ms\n", slowMin)
		fmt.Printf("  Average:          %d ms\n", slowAvg)
		fmt.Printf("  Maximum:          %d ms\n", slowMax)
		fmt.Printf("  p50:              %d ms\n", slowP50)
		fmt.Printf("  p90:              %d ms\n", slowP90)
		fmt.Printf("  p99:              %d ms\n", slowP99)
		fmt.Println()
	}

	fmt.Println("Throughput:")
	rps := float64(successRequests) / duration.Seconds()
	fastRps := float64(fastRequests) / duration.Seconds()
	slowRps := float64(slowRequests) / duration.Seconds()
	fmt.Printf("  Overall:          %.2f req/s\n", rps)
	if len(fastLatencies) > 0 {
		fmt.Printf("  Fast Clients:     %.2f req/s\n", fastRps)
	}
	if len(slowLatencies) > 0 {
		fmt.Printf("  Slow Clients:     %.2f req/s\n", slowRps)
	}

	// Calculate efficiency (actual vs theoretical max)
	if len(fastLatencies) > 0 && fastAvg > 0 {
		theoreticalMaxFastRps := 1000.0 / float64(fastAvg) * float64(config.FastClients)
		actualFastRps := fastRps
		efficiency := (actualFastRps / theoreticalMaxFastRps) * 100
		fmt.Printf("  Fast Client Efficiency: %.1f%% (actual vs theoretical max)\n", efficiency)
		if efficiency < 50 {
			fmt.Printf("     ⚠️  Low efficiency suggests connection hogging!\n")
		}
	}
	fmt.Println()

	// Performance assessment
	fmt.Println("Performance Assessment:")

	// Use fast client p50 for assessment if available
	assessP50 := p50
	if len(fastLatencies) > 0 {
		assessP50 = fastP50
	}

	if assessP50 < 50 {
		fmt.Println("  ✅ p50: Excellent - under 50ms")
	} else if assessP50 < 100 {
		fmt.Println("  ✅ p50: Good - under 100ms")
	} else if assessP50 < 200 {
		fmt.Println("  ⚠️  p50: Fair - under 200ms")
	} else {
		fmt.Println("  ❌ p50: Poor - over 200ms")
	}

	// Use fast client p99 for assessment if available
	assessP99 := p99
	if len(fastLatencies) > 0 {
		assessP99 = fastP99
	}

	if assessP99 < 200 {
		fmt.Println("  ✅ p99: Excellent - under 200ms")
	} else if assessP99 < 500 {
		fmt.Println("  ✅ p99: Good - under 500ms")
	} else if assessP99 < 1000 {
		fmt.Println("  ⚠️  p99: Fair - under 1s")
	} else {
		fmt.Println("  ❌ p99: Poor - over 1s")
	}

	successRate := float64(successRequests) / float64(totalRequests) * 100
	if successRate >= 99.9 {
		fmt.Println("  ✅ Success rate: Excellent - 99.9%+")
	} else if successRate >= 99 {
		fmt.Println("  ✅ Success rate: Good - 99%+")
	} else if successRate >= 95 {
		fmt.Println("  ⚠️  Success rate: Fair - 95%+")
	} else {
		fmt.Println("  ❌ Success rate: Poor - below 95%")
	}

	// Special analysis for hog test
	if config.ConnectionHogTest && len(fastLatencies) > 0 {
		fmt.Println()
		fmt.Println("Connection Hogging Analysis:")
		if fastP99 > 500 {
			fmt.Println("  ❌ DETECTED: Slow clients are significantly impacting fast clients!")
			fmt.Printf("     Fast client p99 latency: %d ms (should be <200ms)\n", fastP99)
			fmt.Println("     This indicates connection pool exhaustion or resource contention.")
		} else if fastP99 > 200 {
			fmt.Println("  ⚠️  WARNING: Some impact detected from slow clients")
			fmt.Printf("     Fast client p99 latency: %d ms\n", fastP99)
			fmt.Println("     Consider implementing connection limits or timeouts.")
		} else {
			fmt.Println("  ✅ Server handles slow clients well - fast clients unaffected")
			fmt.Printf("     Fast client p99 latency: %d ms\n", fastP99)
		}
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
