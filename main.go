package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"go-localization-large-backend/pkg/model"
)

// Payload holds the name and content of a payload file
type Payload struct {
	Name    string
	Content string
}

var payloads []Payload

func init() {
	// Load all payload files from the payloads directory
	payloadDir := "payloads"
	entries, err := os.ReadDir(payloadDir)
	if err != nil {
		log.Fatalf("Failed to read payloads directory: %v", err)
	}

	// Collect and sort payload names for deterministic ordering
	var payloadNames []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			payloadNames = append(payloadNames, entry.Name())
		}
	}
	sort.Strings(payloadNames)

	// Load each payload
	for _, name := range payloadNames {
		payloadPath := filepath.Join(payloadDir, name)
		content, err := os.ReadFile(payloadPath)
		if err != nil {
			log.Printf("Warning: failed to load %s: %v", payloadPath, err)
			continue
		}

		// Parse JSON to check structure
		var parsed map[string]interface{}
		if err := json.Unmarshal(content, &parsed); err != nil {
			log.Printf("Warning: %s contains invalid JSON: %v", payloadPath, err)
			continue
		}

		// Check if this JSON has a "payloads" array
		if payloadsArray, ok := parsed["payloads"].([]interface{}); ok {
			// Extract individual payloads from the array
			log.Printf("Found payloads array in %s with %d items", name, len(payloadsArray))
			for i, item := range payloadsArray {
				itemBytes, err := json.Marshal(item)
				if err != nil {
					log.Printf("Warning: failed to marshal payload %d from %s: %v", i, name, err)
					continue
				}
				payloads = append(payloads, Payload{
					Name:    fmt.Sprintf("%s[%d]", name, i),
					Content: string(itemBytes),
				})
			}
			log.Printf("Loaded %d payloads from %s", len(payloadsArray), name)
		} else {
			// No "payloads" array, use the whole file as one payload
			payloads = append(payloads, Payload{
				Name:    name,
				Content: string(content),
			})
			log.Printf("Loaded payload: %s (%d bytes)", name, len(content))
		}
	}

	if len(payloads) == 0 {
		log.Fatal("No payloads loaded")
	}
	log.Printf("Loaded %d payloads total", len(payloads))
}

func main() {
	// Create a new Fiber instance with slow client protections
	app := fiber.New(fiber.Config{
		AppName:               "Go Localization Backend",
		DisableStartupMessage: false,

		// Slow client protection: timeouts prevent clients from holding connections
		// indefinitely. These are critical for preventing "slowloris" style attacks
		// and resource exhaustion from slow network clients.

		// ReadTimeout: Max time to read the full request including body.
		// Protects against slow request senders.
		ReadTimeout: 5 * time.Second,

		// WriteTimeout: Max time to write the full response.
		// This is the KEY protection against slow clients - if a client can't
		// receive our ~1MB payload within this time, we close the connection
		// rather than letting them hog server resources.
		WriteTimeout: 10 * time.Second,

		// IdleTimeout: Max time to wait for the next request on a keep-alive connection.
		// Frees up connections from idle clients.
		IdleTimeout: 30 * time.Second,

		// Concurrency: Max concurrent connections. Provides a hard cap on resource usage.
		// Default is 256*1024 which is very high - we set a reasonable limit.
		Concurrency: 10000,

		// BodyLimit: Max request body size (1MB). Prevents memory exhaustion from
		// clients sending huge request bodies.
		BodyLimit: 1 * 1024 * 1024,
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Health check endpoint
	app.Get("/health", healthCheck)

	// Experiment endpoint
	app.Post("/experiment", experiment)

	// Start server
	log.Fatal(app.Listen(":3000"))
}

// Health check handler
func healthCheck(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  "ok",
		"message": "Server is running",
	})
}

// Experiment handler
func experiment(c *fiber.Ctx) error {
	var req model.Request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "userId is required",
		})
	}

	// Deterministically assign a payload based on UserID hash
	payload := getPayloadForUser(req.UserID)

	response := model.Response{
		ExperimentID:        "exp-localization-v1",
		SelectedPayloadName: payload.Name,
		Payload:             json.RawMessage(payload.Content),
	}

	return c.JSON(response)
}

// getPayloadForUser returns a deterministic payload for a given user ID
func getPayloadForUser(userID string) Payload {
	h := fnv.New32a()
	h.Write([]byte(userID))
	index := int(h.Sum32()) % len(payloads)
	return payloads[index]
}
