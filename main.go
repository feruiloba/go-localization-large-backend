package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

var dummyPayload []byte

func init() {
	var err error
	// Load the 1MB payload from the payloads directory
	payloadPath := filepath.Join("payloads", "localization_dummy_3.json")
	dummyPayload, err = os.ReadFile(payloadPath)
	if err != nil {
		log.Printf("⚠️  Error loading payload from %s: %v", payloadPath, err)
		// Fallback to empty JSON object if file fails to load
		dummyPayload = []byte("{}")
	} else {
		log.Printf("✅ Loaded dummy payload (%d bytes)", len(dummyPayload))
	}
}

func main() {
	// Create a new Fiber instance
	app := fiber.New(fiber.Config{
		AppName: "Go Localization Backend",
		// Disable startup message for cleaner output during tests
		DisableStartupMessage: false,
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

	c.Set("Content-Type", "application/json")
	return c.Send(dummyPayload)
}
