package main

import (
	"context"
	"fmt"
	"io" // io 패키지 임포트
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8" // utf8 패키지 임포트

	firebase "firebase.google.com/go/v4"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/htmlindex" // htmlindex 패키지 임포트
	"golang.org/x/text/transform"          // transform 패키지 임포트
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewsArticle struct represents a news article.
type NewsArticle struct {
	Title       string    `firestore:"title"`
	Summary     string    `firestore:"summary"`
	Content     string    `firestore:"content"`
	Source      string    `firestore:"source"`
	URL         string    `firestore:"url"`
	CollectedAt time.Time `firestore:"collectedAt"`
}

// Firestore client instance
var firestoreApp *firebase.App 

// 크롤링 관련 상수 정의
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
	return &NewsCrawlerService{Config: cfg}
}

// articleExistsInFirestore checks if an article with the given URL exists in Firestore.
func (s *NewsCrawlerService) articleExistsInFirestore(url string) (bool, error) {
	if firestoreApp == nil {
		return false, fmt.Errorf("Firestore client not initialized")
	}
	ctx := context.Background()
	client, err := firestoreApp.Firestore(ctx) 
	if err != nil {
		return false, fmt.Errorf("error getting Firestore client: %v", err)
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
			return false, nil 
		}
		return false, fmt.Errorf("error checking if article exists in Firestore: %v", err)
	}
	return docSnap.Exists(), nil
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
		log.Printf("Firestore 저장 시도 실패: %s. 원본 에러: %v", article.Title, err)
		// Go 1.20에서는 min이 내장 함수가 아니므로 직접 계산
		contentPreviewLength := 100
		if len(article.Content) < contentPreviewLength {
			contentPreviewLength = len(article.Content)
		}
		log.Printf("유효하지 않은 UTF-8 문자열 가능성 확인: Title='%s', Summary='%s', Content (일부)='%s', Source='%s'", 
            article.Title, article.Summary, article.Content[:contentPreviewLength], article.Source)
		return fmt.Errorf("error saving article to Firestore: %v", err)
	}
	log.Printf("Firestore에 기사 저장 완료: %s", article.Title)
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


// CrawlNaverFinanceNews performs the crawling operation.
func (s *NewsCrawlerService) CrawlNaverFinanceNews(pages int) ([]NewsArticle, error) {
	allNews := []NewsArticle{}
	log.Printf("네이버 금융 뉴스 %d 페이지 수집을 시작합니다...", pages)

	articleIDPattern := regexp.MustCompile(`article_id=(\d+)`)
	officeIDPattern := regexp.MustCompile(`office_id=(\d+)`)

	for pageNum := 1; pageNum <= pages; pageNum++ {
		pageURL := fmt.Sprintf("%s?page=%d", s.Config.NaverFinanceBaseURL, pageNum)
		req, err := http.NewRequest("GET", pageURL, nil)
		if err != nil {
			log.Printf("페이지 %d 요청 생성 오류: %v", pageNum, err)
			return allNews, err
		}
		req.Header.Set("User-Agent", s.Config.UserAgent)

		client := &http.Client{Timeout: 10 * time.Second} // Main page timeout 10 seconds
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("페이지 %d 요청 중 오류 발생: %v", pageNum, err)
			log.Println("네트워크 문제 또는 사이트 차단 가능성. 잠시 후 다시 시도하거나 IP 변경을 고려하세요.")
			break // Error, stop crawling
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("페이지 %d HTTP 상태 코드 오류: %d", pageNum, resp.StatusCode)
			break // HTTP error, stop crawling
		}

		// --- Explicitly decode response body based on charset ---
		// Read the entire body first
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("페이지 %d 응답 본문 읽기 오류: %v", pageNum, err)
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

		var reader io.Reader = strings.NewReader(string(bodyBytes)) // Default to raw bytes as UTF-8
		if charset != "utf-8" && charset != "" {
			// Try to get a decoder for the detected charset
			e, err := htmlindex.Get(charset)
			if err == nil && e != nil {
				reader = transform.NewReader(strings.NewReader(string(bodyBytes)), e.NewDecoder())
				log.Printf("페이지 %d: %s 인코딩으로 변환 시도.", pageNum, charset)
			} else {
				log.Printf("페이지 %d: %s 인코딩 디코더를 찾을 수 없거나 오류 발생 (%v). UTF-8로 처리합니다.", pageNum, charset, err)
			}
		}
		// --- End of explicit decoding ---

		doc, err := goquery.NewDocumentFromReader(reader) // Use the decoded reader
		if err != nil {
			log.Printf("페이지 %d HTML 파싱 오류: %v", pageNum, err)
			break // Parsing error, stop crawling
		}

		newsItems := doc.Find("ul.newsList li")
		if newsItems.Length() == 0 {
			log.Printf("페이지 %d에서 뉴스 목록 (ul.newsList li)을 찾을 수 없습니다. 크롤링을 중단합니다.", pageNum)
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
				log.Printf("경고: 필수 뉴스 요소(제목, 요약, 출처, 링크)가 누락되어 스킵합니다. 뉴스 항목 HTML:\n%s", itemHtml)
				return 
			}

			// --- Reconstruct URL for full article content ---
			var fullArticleURL string
			articleIDMatch := articleIDPattern.FindStringSubmatch(originalLink)
			officeIDMatch := officeIDPattern.FindStringSubmatch(originalLink)

			if len(articleIDMatch) > 1 && len(officeIDMatch) > 1 {
				fullArticleURL = fmt.Sprintf("%s/%s/%s", s_crawler.Config.NaverArticleBaseURL, officeIDMatch[1], articleIDMatch[1]) 
			} else {
				log.Printf("경고: article_id 또는 office_id를 추출할 수 없습니다. 원본 링크: %s", originalLink)
				fullArticleURL = "https://finance.naver.com" + originalLink 
			}

			// Check for existence in Firestore to prevent duplicates
			exists, err := s_crawler.articleExistsInFirestore(fullArticleURL) 
			if err != nil {
				log.Printf("Firestore 존재 여부 확인 오류: %v", err)
				return 
			}
			if exists {
				log.Printf("정보: 이미 존재하는 기사입니다. 스킵: %s", fullArticleURL)
				return 
			}

			// --- Fetch full article content with retries ---
			fullContent := summaryText 
			if fullArticleURL != "" {
				for retry := 0; retry < 3; retry++ { 
					reqArticle, err := http.NewRequest("GET", fullArticleURL, nil)
					if err != nil {
						log.Printf("기사 본문 요청 생성 오류: %v", err)
						break
					}
					reqArticle.Header.Set("User-Agent", s_crawler.Config.UserAgent) 

					articleClient := &http.Client{Timeout: ARTICLE_FETCH_TIMEOUT_MS} 
					respArticle, err := articleClient.Do(reqArticle)
					if err != nil {
						log.Printf("기사 본문 로딩 중 오류 발생 (재시도 %d/3): %v - %s", retry+1, fullArticleURL, err)
						if retry < 2 {
							time.Sleep(time.Duration(1+retry) * time.Second) 
						}
						continue 
					}
					defer respArticle.Body.Close()

					if respArticle.StatusCode != http.StatusOK {
						log.Printf("기사 본문 HTTP 상태 코드 오류: %d - %s", respArticle.StatusCode, fullArticleURL)
						break
					}
					
					// --- Explicitly decode article response body ---
					articleBodyBytes, err := io.ReadAll(respArticle.Body)
					if err != nil {
						log.Printf("기사 본문 응답 본문 읽기 오류: %v", err)
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

					var articleReader io.Reader = strings.NewReader(string(articleBodyBytes))
					if articleCharset != "utf-8" && articleCharset != "" {
						e, err := htmlindex.Get(articleCharset)
						if err == nil && e != nil {
							articleReader = transform.NewReader(strings.NewReader(string(articleBodyBytes)), e.NewDecoder())
							log.Printf("기사 본문: %s 인코딩으로 변환 시도.", articleCharset)
						} else {
							log.Printf("기사 본문: %s 인코딩 디코더를 찾을 수 없거나 오류 발생 (%v). UTF-8로 처리합니다.", articleCharset, err)
						}
					}
					// --- End of explicit decoding for article body ---

					articleDoc, err := goquery.NewDocumentFromReader(articleReader) // Use the decoded reader
					if err != nil {
						log.Printf("기사 본문 HTML 파싱 오류: %v - %s", err, fullArticleURL)
						break
					}

					contentDiv := articleDoc.Find("article#dic_area") 
					if contentDiv.Length() > 0 {
						contentDiv.Find("script, iframe, a, strong, em, br, .end_photo_org, .link_text, .byline, .reporter_area, .nbd_im_w, .img_desc").Remove()
						fullContent = strings.TrimSpace(contentDiv.Text())
						break 
					} else {
						log.Printf("경고: 기사 본문 div (article#dic_area)를 찾을 수 없습니다: %s (재구성된 URL)", fullArticleURL)
						break 
					}
				}
			}
            
            // 추출된 모든 문자열을 Firestore 저장 전에 유효한 UTF-8로 클린징
            title = cleanUTF8String(title)
            summaryText = cleanUTF8String(summaryText)
            fullContent = cleanUTF8String(fullContent)
            sourceText = cleanUTF8String(sourceText)
            fullArticleURL = cleanUTF8String(fullArticleURL) 

			newsArticle := NewsArticle{
				Title:       title,
				Summary:     summaryText,
				Content:     fullContent,
				Source:      sourceText,
				URL:         fullArticleURL,
				CollectedAt: time.Now(),
			}

			err = s_crawler.saveArticleToFirestore(newsArticle) 
			if err != nil {
				log.Printf("Firestore 저장 오류: %v", err)
				return 
			}
			allNews = append(allNews, newsArticle)

			time.Sleep(time.Duration(rand.Intn(500)+200) * time.Millisecond) 
		})

		log.Printf("페이지 %d 수집 완료. 현재까지 %d개 기사 수집 및 Firestore 저장.", pageNum, len(allNews))
		time.Sleep(time.Duration(rand.Intn(3)+2) * time.Second) 
	}
	log.Println("뉴스 수집이 완료되었습니다.")
	return allNews, nil
}