package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Check required environment variables
	if os.Getenv("OPENAI_API_KEY") == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Fatal("At least one of OPENAI_API_KEY or ANTHROPIC_API_KEY must be set")
	}

	// Run main application
	// Import and call the main package's setup here
	// For now, this is a placeholder
	log.Println("Starting AI Gateway...")
}

