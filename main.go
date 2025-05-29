package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	// 1. Load configurations
	cfg := LoadConfig()

	// 2. Initialize Firebase Firestore client
	err := InitializeFirestoreClient(cfg.FirebaseServiceAccountKeyPath)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}

	// 3. Create News Crawler Service instance
	crawlerService := NewNewsCrawlerService(cfg)

	// 4. Create Fiber web application
	app := fiber.New()

	// Add logging middleware
	app.Use(logger.New())

	// Add CORS middleware (might not be strictly necessary for a crawler,
	// but kept for development convenience or if other services call this API)
	app.Use(func(c *fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS") // GET is no longer used, can be removed
		c.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept")
		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return c.Next()
	})

	// 5. Define REST API Endpoints

	// News crawling trigger endpoint (for Cloud Scheduler)
	app.Post("/api/schedule/crawl", func(c *fiber.Ctx) error {
		log.Println("HTTP request received to start news crawling...")

		pagesStr := c.Query("pages", "1")
		pages, err := strconv.Atoi(pagesStr)
		if err != nil {
			log.Printf("Invalid 'pages' parameter value: %s. Using default of 1.", pagesStr)
			pages = 1
		}

		if pages <= 0 || pages > 10 {
			log.Printf("Invalid number of pages requested: %d. Limited to 1-10 pages.", pages)
			return c.Status(fiber.StatusBadRequest).SendString("Invalid number of pages requested. Please specify within 1-10 pages.")
		}

		log.Printf("Crawling %d pages.", pages)

		_, err = crawlerService.CrawlNaverFinanceNews(pages)
		if err != nil {
			log.Printf("Error during news crawling operation: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf("Error during news crawling operation: %v", err))
		}
		log.Println("News crawling operation completed via HTTP request.")
		return c.Status(fiber.StatusOK).SendString(fmt.Sprintf("News crawling operation successfully triggered. (Pages crawled: %d)", pages))
	})

	// 6. Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}
	log.Printf("Crawler server starting on port %s...", port)
	log.Fatal(app.Listen(":" + port))
}
