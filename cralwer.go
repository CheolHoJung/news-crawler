package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewsArticle struct represents a news article.
type NewsArticle struct {
	Title       string    `firestore:"title"`
	Summary     string    `firestore:"summary"`
	Content     string    `firestore:"content"`   // Original content
	AISummary   string    `firestore:"aiSummary"` // AI summary (filled by summarization server)
	Source      string    `firestore:"source"`
	URL         string    `firestore:"url"`
	CollectedAt time.Time `firestore:"collectedAt"`
}

// Firestore client instance
var firestoreApp *firebase.App

// Constants related to crawling
const (
	MAX_ARTICLE_FETCH_RETRIES    = 3
	ARTICLE_FETCH_RETRY_DELAY_MS = 1000
	ARTICLE_FETCH_TIMEOUT_MS     = 20 * time.Second
)

// InitializeFirestoreClient initializes the Firebase Firestore client.
func InitializeFirestoreClient(serviceAccountKeyPath string) error {
	ctx := context.Background()
	opt := option.WithCredentialsFile(serviceAccountKeyPath)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return fmt.Errorf("error initializing Firebase app: %v", err)
	}
	firestoreApp = app
	log.Println("Firebase Firestore client initialized successfully.")
	return nil
}

// NewsCrawlerService struct holds the configurations and performs crawling.
type NewsCrawlerService struct {
	Config *Config
}

// NewNewsCrawlerService creates a new NewsCrawlerService instance.
func NewNewsCrawlerService(cfg *Config) *NewsCrawlerService {
	return &NewsCrawlerService{
		Config: cfg,
	}
}

// articleExistsInFirestore checks if an article with the given URL exists in Firestore.
// It returns true if the article exists, the existing NewsArticle object (if found), and an error.
func (s *NewsCrawlerService) articleExistsInFirestore(url string) (bool, *NewsArticle, error) {
	if firestoreApp == nil {
		return false, nil, fmt.Errorf("Firestore client not initialized")
	}
	ctx := context.Background()
	client, err := firestoreApp.Firestore(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("error getting Firestore client: %v", err)
	}
	defer client.Close()

	docID := strings.ReplaceAll(url, "/", "_")
	docID = strings.ReplaceAll(docID, ":", "_")
	docID = strings.ReplaceAll(docID, "?", "_")
	docID = strings.ReplaceAll(docID, "&", "_")
	docID = strings.ReplaceAll(docID, "=", "_")
	docID = strings.ReplaceAll(docID, "#", "_")
	docID = strings.ReplaceAll(docID, "%", "_")
	docID = strings.ReplaceAll(docID, ".", "_")

	if len(docID) > 500 {
		docID = docID[:500]
	}

	docRef := client.Collection("newsArticles").Doc(docID)
	docSnap, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil, nil // Document not found
		}
		return false, nil, fmt.Errorf("error checking if article exists in Firestore: %v", err)
	}

	if docSnap.Exists() {
		var existingArticle NewsArticle
		if err := docSnap.DataTo(&existingArticle); err != nil {
			log.Printf("Warning: Failed to convert existing Firestore document data to NewsArticle: %v", err)
			return true, nil, fmt.Errorf("failed to convert existing article data")
		}
		return true, &existingArticle, nil
	}
	return false, nil, nil
}

// updateArticleAISummaryToEmpty updates an existing article's AISummary field to an empty string.
func (s *NewsCrawlerService) updateArticleAISummaryToEmpty(url string) error {
	if firestoreApp == nil {
		return fmt.Errorf("Firestore client not initialized")
	}
	ctx := context.Background()
	client, err := firestoreApp.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("error getting Firestore client: %v", err)
	}
	defer client.Close()

	docID := strings.ReplaceAll(url, "/", "_")
	docID = strings.ReplaceAll(docID, ":", "_")
	docID = strings.ReplaceAll(docID, "?", "_")
	docID = strings.ReplaceAll(docID, "&", "_")
	docID = strings.ReplaceAll(docID, "=", "_")
	docID = strings.ReplaceAll(docID, "#", "_")
	docID = strings.ReplaceAll(docID, "%", "_")
	docID = strings.ReplaceAll(docID, ".", "_")

	if len(docID) > 500 {
		docID = docID[:500]
	}

	_, err = client.Collection("newsArticles").Doc(docID).Update(ctx, []firestore.Update{
		{Path: "aiSummary", Value: ""},
	})
	if err != nil {
		return fmt.Errorf("error updating existing article's AISummary to empty: %v", err)
	}
	log.Printf("Updated existing article's AISummary to empty: %s", url)
	return nil
}

// saveArticleToFirestore saves a NewsArticle to Firestore.
func (s *NewsCrawlerService) saveArticleToFirestore(article NewsArticle) error {
	if firestoreApp == nil {
		return fmt.Errorf("Firestore client not initialized")
	}
	ctx := context.Background()
	client, err := firestoreApp.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("error getting Firestore client: %v", err)
	}
	defer client.Close()

	docID := strings.ReplaceAll(article.URL, "/", "_")
	docID = strings.ReplaceAll(docID, ":", "_")
	docID = strings.ReplaceAll(docID, "?", "_")
	docID = strings.ReplaceAll(docID, "&", "_")
	docID = strings.ReplaceAll(docID, "=", "_")
	docID = strings.ReplaceAll(docID, "#", "_")
	docID = strings.ReplaceAll(docID, "%", "_")
	docID = strings.ReplaceAll(docID, ".", "_")

	if len(docID) > 500 {
		docID = docID[:500]
	}

	_, err = client.Collection("newsArticles").Doc(docID).Set(ctx, article)
	if err != nil {
		log.Printf("Firestore save attempt failed: %s. Original error: %v", article.Title, err)
		contentPreviewLength := 100
		if len(article.Content) < contentPreviewLength {
			contentPreviewLength = len(article.Content)
		}
		log.Printf("Potential invalid UTF-8 string detected: Title='%s', Summary='%s', Content (partial)='%s', Source='%s'",
			article.Title, article.Summary, article.Content[:contentPreviewLength], article.Source)
		return fmt.Errorf("error saving article to Firestore: %v", err)
	}
	log.Printf("Article saved to Firestore: %s", article.Title)
	return nil
}

// cleanUTF8String ensures the string contains only valid UTF-8 characters.
func cleanUTF8String(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	v := make([]rune, 0, len(s))
	for i, r := range s {
		if r == utf8.RuneError {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size == 1 {
				v = append(v, ' ')
				continue
			}
		}
		v = append(v, r)
	}
	return string(v)
}

// SearchNewsArticles searches for news articles in Firestore based on a keyword.
func (s *NewsCrawlerService) SearchNewsArticles(ctx context.Context, keyword string) ([]NewsArticle, error) {
	if firestoreApp == nil {
		return nil, fmt.Errorf("Firestore client not initialized")
	}
	client, err := firestoreApp.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting Firestore client: %v", err)
	}
	defer client.Close()

	var results []NewsArticle
	iter := client.Collection("newsArticles").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating over Firestore documents: %v", err)
		}

		var article NewsArticle
		if err := doc.DataTo(&article); err != nil {
			log.Printf("Warning: Failed to convert Firestore document data to NewsArticle: %v", err)
			continue
		}

		// Keyword search (case-insensitive)
		lowerKeyword := strings.ToLower(keyword)
		if strings.Contains(strings.ToLower(article.Title), lowerKeyword) ||
			strings.Contains(strings.ToLower(article.Summary), lowerKeyword) ||
			strings.Contains(strings.ToLower(article.Content), lowerKeyword) {
			results = append(results, article)
		}
	}
	return results, nil
}

// CrawlNaverFinanceNews performs the crawling operation.
func (s *NewsCrawlerService) CrawlNaverFinanceNews(pages int) ([]NewsArticle, error) {
	allNews := []NewsArticle{}
	log.Printf("Starting Naver Finance news collection for %d pages...", pages)

	articleIDPattern := regexp.MustCompile(`article_id=(\d+)`)
	officeIDPattern := regexp.MustCompile(`office_id=(\d+)`)

	for pageNum := 1; pageNum <= pages; pageNum++ {
		pageURL := fmt.Sprintf("%s?page=%d", s.Config.NaverFinanceBaseURL, pageNum)
		req, err := http.NewRequest("GET", pageURL, nil)
		if err != nil {
			log.Printf("Error creating request for page %d: %v", pageNum, err)
			return allNews, err
		}
		req.Header.Set("User-Agent", s.Config.UserAgent)

		client := &http.Client{Timeout: 10 * time.Second} // Main page timeout 10 seconds
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error requesting page %d: %v", pageNum, err)
			log.Println("Network issue or site blocking possible. Retrying later or consider changing IP.")
			break // Error, stop crawling
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("HTTP status code error for page %d: %d", pageNum, resp.StatusCode)
			break // HTTP error, stop crawling
		}

		// --- Explicitly decode response body based on charset ---
		// Read the entire body first
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response body for page %d: %v", pageNum, err)
			break
		}

		// Determine charset from Content-Type header
		contentType := resp.Header.Get("Content-Type")
		charset := "utf-8" // Default to UTF-8
		if strings.Contains(contentType, "charset=") {
			parts := strings.Split(contentType, "charset=")
			if len(parts) > 1 {
				charset = strings.ToLower(strings.TrimSpace(parts[1]))
			}
		}

		var reader io.Reader = bytes.NewReader(bodyBytes)
		if charset != "utf-8" && charset != "" {
			e, err := htmlindex.Get(charset)
			if err == nil && e != nil {
				reader = transform.NewReader(bytes.NewReader(bodyBytes), e.NewDecoder())
				log.Printf("Page %d: Attempting to convert using %s encoding.", pageNum, charset)
			} else {
				log.Printf("Page %d: Could not find or error with %s encoding decoder (%v). Processing as UTF-8.", pageNum, charset, err)
			}
		}
		// --- End of explicit decoding ---

		doc, err := goquery.NewDocumentFromReader(reader)
		if err != nil {
			log.Printf("HTML parsing error for page %d: %v", pageNum, err)
			break // Parsing error, stop crawling
		}

		newsItems := doc.Find("ul.newsList li")
		if newsItems.Length() == 0 {
			log.Printf("Could not find news list (ul.newsList li) on page %d. Stopping crawl.", pageNum)
			break
		}

		s_crawler := s

		newsItems.Each(func(i int, s_item *goquery.Selection) {
			select {
			case <-context.Background().Done():
				return
			default:
			}

			// Extract data from each news item
			titleTag := s_item.Find("dd.articleSubject a")
			summaryDdTag := s_item.Find("dd.articleSummary")

			title := strings.TrimSpace(titleTag.Text())
			originalLink, _ := titleTag.Attr("href")

			var summaryText string
			var sourceText string

			if summaryDdTag.Length() > 0 {
				sourceSpan := summaryDdTag.Find("span.press")
				if sourceSpan.Length() > 0 {
					sourceText = strings.TrimSpace(sourceSpan.Text())
					sourceSpan.Remove()
				}

				wdateSpan := summaryDdTag.Find("span.wdate")
				if wdateSpan.Length() > 0 {
					wdateSpan.Remove()
				}

				barSpan := summaryDdTag.Find("span.bar")
				if barSpan.Length() > 0 {
					barSpan.Remove()
				}

				summaryText = strings.TrimSpace(summaryDdTag.Text())
			}

			// Validate extracted data
			if title == "" || summaryText == "" || sourceText == "" || originalLink == "" {
				itemHtml, _ := goquery.OuterHtml(s_item)
				log.Printf("Warning: Missing required news elements (title, summary, source, link). News item HTML:\n%s", itemHtml)
				return
			}

			// --- Reconstruct URL for full article content ---
			var fullArticleURL string
			articleIDMatch := articleIDPattern.FindStringSubmatch(originalLink)
			officeIDMatch := officeIDPattern.FindStringSubmatch(originalLink)

			if len(articleIDMatch) > 1 && len(officeIDMatch) > 1 {
				fullArticleURL = fmt.Sprintf("%s/%s/%s", s_crawler.Config.NaverArticleBaseURL, officeIDMatch[1], articleIDMatch[1])
			} else {
				log.Printf("Warning: Could not extract article_id or office_id. Original link: %s", originalLink)
				fullArticleURL = "https://finance.naver.com" + originalLink
			}

			// Check for existence in Firestore to prevent duplicates
			exists, existingArticle, err := s_crawler.articleExistsInFirestore(fullArticleURL)
			if err != nil {
				log.Printf("Firestore existence check error: %v", err)
				return
			}
			if exists {
				// If article exists, check if AISummary is missing or empty.
				// If AISummary is missing or empty, update it to "".
				if existingArticle != nil && existingArticle.AISummary == "" {
					err := s_crawler.updateArticleAISummaryToEmpty(fullArticleURL)
					if err != nil {
						log.Printf("Warning: Failed to update existing article's AISummary to empty: %v", err)
					}
				}
				log.Printf("Info: Article already exists. Skipping new save for: %s", fullArticleURL)
				return
			}

			// --- Fetch full article content with retries ---
			fullContent := summaryText
			if fullArticleURL != "" {
				for retry := 0; retry < 3; retry++ {
					reqArticle, err := http.NewRequest("GET", fullArticleURL, nil)
					if err != nil {
						log.Printf("Error creating article content request: %v", err)
						break
					}
					reqArticle.Header.Set("User-Agent", s_crawler.Config.UserAgent)

					articleClient := &http.Client{Timeout: ARTICLE_FETCH_TIMEOUT_MS}
					respArticle, err := articleClient.Do(reqArticle)
					if err != nil {
						log.Printf("Error loading article content (retry %d/3): %v - %s", retry+1, fullArticleURL, err)
						if retry < 2 {
							time.Sleep(time.Duration(1+retry) * time.Second)
						}
						continue
					}
					defer respArticle.Body.Close()

					if respArticle.StatusCode != http.StatusOK {
						log.Printf("Article content HTTP status code error: %d - %s", respArticle.StatusCode, fullArticleURL)
						break
					}

					// --- Explicitly decode article response body ---
					articleBodyBytes, err := io.ReadAll(respArticle.Body)
					if err != nil {
						log.Printf("Error reading article response body: %v", err)
						break
					}

					articleContentType := respArticle.Header.Get("Content-Type")
					articleCharset := "utf-8" // Default to UTF-8
					if strings.Contains(articleContentType, "charset=") {
						parts := strings.Split(articleContentType, "charset=")
						if len(parts) > 1 {
							articleCharset = strings.ToLower(strings.TrimSpace(parts[1]))
						}
					}

					var articleReader io.Reader = bytes.NewReader(articleBodyBytes)
					if articleCharset != "utf-8" && articleCharset != "" {
						e, err := htmlindex.Get(articleCharset)
						if err == nil && e != nil {
							articleReader = transform.NewReader(bytes.NewReader(articleBodyBytes), e.NewDecoder())
							log.Printf("Article content: Attempting to convert using %s encoding.", articleCharset)
						} else {
							log.Printf("Article content: Could not find or error with %s encoding decoder (%v). Processing as UTF-8.", articleCharset, err)
						}
					}
					// --- End of explicit decoding for article body ---

					articleDoc, err := goquery.NewDocumentFromReader(articleReader)
					if err != nil {
						log.Printf("Article content HTML parsing error: %v - %s", err, fullArticleURL)
						break
					}

					contentDiv := articleDoc.Find("article#dic_area")
					if contentDiv.Length() > 0 {
						contentDiv.Find("script, iframe, a, strong, em, br, .end_photo_org, .link_text, .byline, .reporter_area, .nbd_im_w, .img_desc").Remove()
						fullContent = strings.TrimSpace(contentDiv.Text())
						break
					} else {
						log.Printf("Warning: Could not find article body div (article#dic_area): %s (reconstructed URL)", fullArticleURL)
						break
					}
				}
			}

			// Clean all extracted strings for valid UTF-8 before saving to Firestore
			title = cleanUTF8String(title)
			summaryText = cleanUTF8String(summaryText)
			fullContent = cleanUTF8String(fullContent)
			sourceText = cleanUTF8String(sourceText)
			fullArticleURL = cleanUTF8String(fullArticleURL)

			newsArticle := NewsArticle{
				Title:       title,
				Summary:     summaryText,
				Content:     fullContent,
				AISummary:   "", // Crawler explicitly sets AI summary to empty.
				Source:      sourceText,
				URL:         fullArticleURL,
				CollectedAt: time.Now(),
			}

			err = s_crawler.saveArticleToFirestore(newsArticle)
			if err != nil {
				log.Printf("Firestore save error: %v", err)
				return
			}
			allNews = append(allNews, newsArticle)

			time.Sleep(time.Duration(rand.Intn(500)+200) * time.Millisecond)
		})

		log.Printf("Page %d collection complete. %d articles collected and saved to Firestore so far.", pageNum, len(allNews))
		time.Sleep(time.Duration(rand.Intn(3)+2) * time.Second)
	}
	log.Println("News collection complete.")
	return allNews, nil
}
