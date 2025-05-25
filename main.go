package main

import (
	"fmt"
	"log"
	"os"
	"strconv" // strconv 패키지 임포트

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	// 1. 설정 로드
	cfg := LoadConfig()

	// 2. Firebase Firestore 클라이언트 초기화
	err := InitializeFirestoreClient(cfg.FirebaseServiceAccountKeyPath)
	if err != nil {
		log.Fatalf("Firebase 초기화 실패: %v", err)
	}

	// 3. 뉴스 크롤러 서비스 인스턴스 생성
	crawlerService := NewNewsCrawlerService(cfg)

	// 4. Fiber 웹 애플리케이션 생성
	app := fiber.New()

	// 로깅 미들웨어 추가
	app.Use(logger.New())

	// 5. REST API 엔드포인트 정의
	// POST 요청을 받는 /api/schedule/crawl 엔드포인트
	// GCP Cloud Scheduler와 같은 외부 스케줄러가 이 엔드포인트를 호출합니다.
	app.Post("/api/schedule/crawl", func(c *fiber.Ctx) error {
		log.Println("HTTP 요청을 통해 뉴스 크롤링을 시작합니다...")

		// 'pages' 쿼리 파라미터 읽기 (기본값은 1)
		pagesStr := c.Query("pages", "1") // 쿼리 파라미터 "pages"를 읽고, 없으면 기본값 "1"
		pages, err := strconv.Atoi(pagesStr) // 문자열을 정수로 변환
		if err != nil {
			log.Printf("잘못된 'pages' 파라미터 값: %s. 기본값 1을 사용합니다.", pagesStr)
			pages = 1 // 변환 실패 시 기본값 1 사용
		}

		// 페이지 수 제한 (과도한 요청 방지)
		if pages <= 0 || pages > 10 { // 1페이지 이상, 10페이지 이하로 제한 (조정 가능)
			log.Printf("유효하지 않은 페이지 수 요청: %d. 1~10 페이지 범위 내로 제한합니다.", pages)
			return c.Status(fiber.StatusBadRequest).SendString("유효하지 않은 페이지 수 요청. 1~10 페이지 범위 내로 지정해주세요.")
		}

		log.Printf("크롤링할 페이지 수: %d", pages)

		_, err = crawlerService.CrawlNaverFinanceNews(pages) // 파라미터로 받은 페이지 수 전달
		if err != nil {
			log.Printf("뉴스 크롤링 작업 중 오류 발생: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf("뉴스 크롤링 작업 중 오류 발생: %v", err))
		}
		log.Println("HTTP 요청을 통한 뉴스 크롤링 작업 완료.")
		return c.Status(fiber.StatusOK).SendString(fmt.Sprintf("뉴스 크롤링 작업이 성공적으로 트리거되었습니다. (크롤링 페이지: %d)", pages))
	})

	// 6. 서버 시작
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // 기본 포트
	}
	log.Printf("서버가 포트 %s에서 시작됩니다...", port)
	log.Fatal(app.Listen(":" + port))
}