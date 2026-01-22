package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type Request struct {
	UserID string `json:"userId"`
}

type Response struct {
	ExperimentID        string `json:"experimentId"`
	SelectedPayloadName string `json:"selectedPayloadName"`
	Payload             string `json:"payload"`
}

type UserAllocation struct {
	UserID       string
	PayloadName  string
	RequestCount int
	Consistent   bool // true if all requests returned the same payload
}

type TestResults struct {
	TotalUsers            int
	TotalRequests         int
	SuccessfulRequests    int
	FailedRequests        int
	ConsistentUsers       int
	InconsistentUsers     int
	PayloadDistribution   map[string]int
	UserAllocations       []UserAllocation
	InconsistentDetails   []string
	TestDuration          time.Duration
	RequestsPerSecond     float64
	AllocationConsistency float64
}

func main() {
	serverURL := flag.String("url", "http://localhost:3000", "Server URL")
	numUsers := flag.Int("users", 100, "Number of unique users to test")
	requestsPerUser := flag.Int("requests", 5, "Number of requests per user")
	concurrency := flag.Int("concurrency", 10, "Number of concurrent workers")
	outputFile := flag.String("output", "allocation_test_results.md", "Output file for results")
	flag.Parse()

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🧪 A/B Allocation Verification Test")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Server URL: %s\n", *serverURL)
	fmt.Printf("Users: %d\n", *numUsers)
	fmt.Printf("Requests per user: %d\n", *requestsPerUser)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Output file: %s\n", *outputFile)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Check server health
	if !checkHealth(*serverURL) {
		fmt.Println("❌ Server health check failed. Is the server running?")
		os.Exit(1)
	}
	fmt.Println("✅ Server health check passed")
	fmt.Println()

	// Generate user IDs
	userIDs := make([]string, *numUsers)
	for i := 0; i < *numUsers; i++ {
		userIDs[i] = uuid.New().String()
	}

	// Run the allocation test
	results := runAllocationTest(*serverURL, userIDs, *requestsPerUser, *concurrency)

	// Print summary to console
	printSummary(results)

	// Write detailed results to file
	if err := writeResults(*outputFile, results); err != nil {
		fmt.Printf("❌ Failed to write results: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n✅ Detailed results written to %s\n", *outputFile)
}

func checkHealth(serverURL string) bool {
	resp, err := http.Get(serverURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func runAllocationTest(serverURL string, userIDs []string, requestsPerUser, concurrency int) TestResults {
	fmt.Println("Running allocation test...")

	startTime := time.Now()

	// Track allocations per user
	userPayloads := make(map[string]map[string]int) // userID -> payloadName -> count
	var mu sync.Mutex

	var totalRequests atomic.Int64
	var successRequests atomic.Int64
	var failedRequests atomic.Int64

	// Create work channel
	type work struct {
		userID string
	}
	workChan := make(chan work, len(userIDs)*requestsPerUser)

	// Fill work channel
	for _, userID := range userIDs {
		for i := 0; i < requestsPerUser; i++ {
			workChan <- work{userID: userID}
		}
	}
	close(workChan)

	// Create worker pool
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 10 * time.Second}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workChan {
				totalRequests.Add(1)

				payload, err := makeRequest(client, serverURL+"/experiment", w.userID)
				if err != nil {
					failedRequests.Add(1)
					continue
				}

				successRequests.Add(1)

				mu.Lock()
				if userPayloads[w.userID] == nil {
					userPayloads[w.userID] = make(map[string]int)
				}
				userPayloads[w.userID][payload]++
				mu.Unlock()
			}
		}()
	}

	// Progress monitoring
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				total := totalRequests.Load()
				expected := int64(len(userIDs) * requestsPerUser)
				pct := float64(total) / float64(expected) * 100
				fmt.Printf("\r   Progress: %d/%d (%.1f%%)", total, expected, pct)
			}
		}
	}()

	wg.Wait()
	close(done)
	fmt.Println()

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Analyze results
	results := analyzeResults(userPayloads, requestsPerUser, duration,
		int(totalRequests.Load()), int(successRequests.Load()), int(failedRequests.Load()))

	return results
}

func makeRequest(client *http.Client, url, userID string) (string, error) {
	reqBody := Request{UserID: userID}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}

	return response.SelectedPayloadName, nil
}

func analyzeResults(userPayloads map[string]map[string]int, requestsPerUser int, duration time.Duration,
	totalReqs, successReqs, failedReqs int) TestResults {

	results := TestResults{
		TotalUsers:          len(userPayloads),
		TotalRequests:       totalReqs,
		SuccessfulRequests:  successReqs,
		FailedRequests:      failedReqs,
		PayloadDistribution: make(map[string]int),
		TestDuration:        duration,
	}

	if duration.Seconds() > 0 {
		results.RequestsPerSecond = float64(successReqs) / duration.Seconds()
	}

	for userID, payloads := range userPayloads {
		// Count total requests for this user
		totalForUser := 0
		for _, count := range payloads {
			totalForUser += count
		}

		// Check consistency - user should have only one payload
		consistent := len(payloads) == 1

		// Get the primary payload (most common if inconsistent)
		var primaryPayload string
		maxCount := 0
		for payload, count := range payloads {
			if count > maxCount {
				maxCount = count
				primaryPayload = payload
			}
		}

		// Update distribution
		results.PayloadDistribution[primaryPayload]++

		allocation := UserAllocation{
			UserID:       userID,
			PayloadName:  primaryPayload,
			RequestCount: totalForUser,
			Consistent:   consistent,
		}
		results.UserAllocations = append(results.UserAllocations, allocation)

		if consistent {
			results.ConsistentUsers++
		} else {
			results.InconsistentUsers++
			// Record inconsistency details
			var payloadList []string
			for payload, count := range payloads {
				payloadList = append(payloadList, fmt.Sprintf("%s(%d)", payload, count))
			}
			results.InconsistentDetails = append(results.InconsistentDetails,
				fmt.Sprintf("User %s received multiple payloads: %s", userID, strings.Join(payloadList, ", ")))
		}
	}

	if results.TotalUsers > 0 {
		results.AllocationConsistency = float64(results.ConsistentUsers) / float64(results.TotalUsers) * 100
	}

	return results
}

func printSummary(results TestResults) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 Test Summary")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Test Duration: %s\n", results.TestDuration.Round(time.Millisecond))
	fmt.Printf("Throughput: %.2f req/s\n", results.RequestsPerSecond)
	fmt.Println()

	fmt.Println("Request Statistics:")
	fmt.Printf("  Total Requests: %d\n", results.TotalRequests)
	fmt.Printf("  Successful: %d\n", results.SuccessfulRequests)
	fmt.Printf("  Failed: %d\n", results.FailedRequests)
	fmt.Println()

	fmt.Println("Allocation Consistency:")
	fmt.Printf("  Total Users: %d\n", results.TotalUsers)
	fmt.Printf("  Consistent Users: %d\n", results.ConsistentUsers)
	fmt.Printf("  Inconsistent Users: %d\n", results.InconsistentUsers)
	fmt.Printf("  Consistency Rate: %.2f%%\n", results.AllocationConsistency)
	fmt.Println()

	if results.AllocationConsistency == 100 {
		fmt.Println("✅ PASS: All users received consistent payload assignments!")
	} else {
		fmt.Println("❌ FAIL: Some users received inconsistent payload assignments!")
	}

	fmt.Println()
	fmt.Println("Payload Distribution:")
	// Sort payloads for consistent output
	var payloadNames []string
	for name := range results.PayloadDistribution {
		payloadNames = append(payloadNames, name)
	}
	sort.Strings(payloadNames)

	for _, name := range payloadNames {
		count := results.PayloadDistribution[name]
		pct := float64(count) / float64(results.TotalUsers) * 100
		fmt.Printf("  %s: %d users (%.1f%%)\n", name, count, pct)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func writeResults(filename string, results TestResults) error {
	var sb strings.Builder

	sb.WriteString("# A/B Allocation Test Results\n\n")
	sb.WriteString(fmt.Sprintf("**Test Date:** %s\n\n", time.Now().Format(time.RFC3339)))

	sb.WriteString("## Test Configuration\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Users:** %d\n", results.TotalUsers))
	sb.WriteString(fmt.Sprintf("- **Total Requests:** %d\n", results.TotalRequests))
	sb.WriteString(fmt.Sprintf("- **Test Duration:** %s\n", results.TestDuration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("- **Throughput:** %.2f req/s\n\n", results.RequestsPerSecond))

	sb.WriteString("## Request Statistics\n\n")
	sb.WriteString(fmt.Sprintf("| Metric | Value |\n"))
	sb.WriteString(fmt.Sprintf("|--------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| Total Requests | %d |\n", results.TotalRequests))
	sb.WriteString(fmt.Sprintf("| Successful | %d |\n", results.SuccessfulRequests))
	sb.WriteString(fmt.Sprintf("| Failed | %d |\n", results.FailedRequests))
	successRate := float64(results.SuccessfulRequests) / float64(results.TotalRequests) * 100
	sb.WriteString(fmt.Sprintf("| Success Rate | %.2f%% |\n\n", successRate))

	sb.WriteString("## Allocation Consistency\n\n")
	sb.WriteString(fmt.Sprintf("| Metric | Value |\n"))
	sb.WriteString(fmt.Sprintf("|--------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| Total Users | %d |\n", results.TotalUsers))
	sb.WriteString(fmt.Sprintf("| Consistent Users | %d |\n", results.ConsistentUsers))
	sb.WriteString(fmt.Sprintf("| Inconsistent Users | %d |\n", results.InconsistentUsers))
	sb.WriteString(fmt.Sprintf("| **Consistency Rate** | **%.2f%%** |\n\n", results.AllocationConsistency))

	if results.AllocationConsistency == 100 {
		sb.WriteString("### ✅ PASS\n\n")
		sb.WriteString("All users received consistent payload assignments across multiple requests.\n")
		sb.WriteString("The A/B testing implementation is **deterministic** and working correctly.\n\n")
	} else {
		sb.WriteString("### ❌ FAIL\n\n")
		sb.WriteString("Some users received inconsistent payload assignments.\n\n")
		sb.WriteString("**Inconsistency Details:**\n\n")
		for _, detail := range results.InconsistentDetails {
			sb.WriteString(fmt.Sprintf("- %s\n", detail))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Payload Distribution\n\n")
	sb.WriteString("This shows how users are distributed across the different payload variants:\n\n")
	sb.WriteString("| Payload | Users | Percentage |\n")
	sb.WriteString("|---------|-------|------------|\n")

	// Sort payloads for consistent output
	var payloadNames []string
	for name := range results.PayloadDistribution {
		payloadNames = append(payloadNames, name)
	}
	sort.Strings(payloadNames)

	for _, name := range payloadNames {
		count := results.PayloadDistribution[name]
		pct := float64(count) / float64(results.TotalUsers) * 100
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% |\n", name, count, pct))
	}
	sb.WriteString("\n")

	// Add sample user allocations
	sb.WriteString("## Sample User Allocations\n\n")
	sb.WriteString("First 20 users and their assigned payloads:\n\n")
	sb.WriteString("| User ID | Payload | Requests | Consistent |\n")
	sb.WriteString("|---------|---------|----------|------------|\n")

	// Sort by user ID for consistent output
	sort.Slice(results.UserAllocations, func(i, j int) bool {
		return results.UserAllocations[i].UserID < results.UserAllocations[j].UserID
	})

	maxSamples := 20
	if len(results.UserAllocations) < maxSamples {
		maxSamples = len(results.UserAllocations)
	}
	for i := 0; i < maxSamples; i++ {
		alloc := results.UserAllocations[i]
		consistentStr := "✅"
		if !alloc.Consistent {
			consistentStr = "❌"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n",
			alloc.UserID, alloc.PayloadName, alloc.RequestCount, consistentStr))
	}

	return os.WriteFile(filename, []byte(sb.String()), 0644)
}
