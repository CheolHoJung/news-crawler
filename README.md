# Go News Crawler Backend

This is a Go-based web crawler and API backend for the **PulseSignal** application. Its primary purpose is to fetch news articles from Naver Finance, store them in Google Cloud Firestore, and provide an API for searching these articles.

## Table of Contents

- [Purpose](#purpose)
- [Setup and Installation](#setup-and-installation)
- [Running the Application](#running-the-application)
- [API Endpoints](#api-api-endpoints)

## Purpose

This backend serves as the data ingestion and retrieval layer for the PulseSignal news application.
* It **crawls** news articles from Naver Finance.
* It **stores** the crawled articles (including full content) in Google Cloud Firestore.
* It provides a **REST API** for frontend applications to search and retrieve the stored news articles.
* It is designed to be deployed as a **serverless** application (e.g., on Google Cloud Run) and triggered by an external scheduler (e.g., GCP Cloud Scheduler).

## Setup and Installation

### Prerequisites

* **Go:** Version 1.21 or higher (recommended: 1.22)
* **Firebase Project:** A Google Cloud / Firebase project with Firestore enabled.
* **Service Account Key:** A Firebase service account key JSON file.

### Steps

1.  **Clone the repository:**
    ```bash
    git clone [https://github.com/your-username/go-news-crawler.git](https://github.com/your-username/go-news-crawler.git) # Replace with your actual repo URL
    cd go-news-crawler
    ```
2.  **Firebase Service Account Key:**
    * Download your Firebase service account key JSON file from the [Firebase Console](https://console.firebase.google.com/) (Project settings ⚙️ > Service accounts > Generate new private key).
    * **Rename** this file to `firebase-service-account-key.json` and place it in your project's **root directory**.
    * **CRITICAL:** Add `firebase-service-account-key.json` to your `.gitignore` file to prevent it from being committed to version control.
3.  **Initialize Go modules and download dependencies:**
    ```bash
    go mod tidy
    ```

## Running the Application

1.  **Ensure your `firebase-service-account-key.json` is in the project root.**
2.  **Run the Go application:**
    ```bash
    go run .
    ```
    The application will start a web server, by default on port `8080`.

## API Endpoints

Once the application is running, you can interact with it via its API endpoints.

### 1. Trigger News Crawling (POST)

This endpoint initiates the news crawling process.

* **URL:** `/api/schedule/crawl`
* **Method:** `POST`
* **Query Parameter:** `pages` (optional, default: 1, max: 10) - Number of pages to crawl.
* **Example:** `curl -X POST "http://localhost:8080/api/schedule/crawl?pages=1"`