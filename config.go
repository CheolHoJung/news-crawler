package main

import (
	"log"
	"os"
)

// Config struct holds application configurations.
type Config struct {
	FirebaseServiceAccountKeyPath string
	NaverFinanceBaseURL           string
	NaverArticleBaseURL           string
	UserAgent                     string
}

// LoadConfig loads configurations from environment variables or defaults.
func LoadConfig() *Config {
	keyPath := os.Getenv("FIREBASE_SERVICE_ACCOUNT_KEY_PATH")
	if keyPath == "" {
		// Default value for development environment (change to your actual path)
		// For example, if you place serviceAccountKey.json in the project root
		keyPath = "firebase-service-account-key.json"
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			// If the file is not found, print a more specific error message and exit
			log.Fatalf("Environment variable FIREBASE_SERVICE_ACCOUNT_KEY_PATH is not set, and default file %s was not found. Please set the correct path to your Firebase service account key file.", keyPath)
		}
	}

	// Default User-Agent if not set
	userAgent := os.Getenv("USER_AGENT")
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
	}

	return &Config{
		FirebaseServiceAccountKeyPath: keyPath,
		NaverFinanceBaseURL:           "https://finance.naver.com/news/mainnews.naver",
		NaverArticleBaseURL:           "https://n.news.naver.com/mnews/article",
		UserAgent:                     userAgent,
	}
}
