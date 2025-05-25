package main

import (
	"log"
	"os"
	// filepath 임포트 (필요시)
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
		// 개발 환경에서 기본값 설정 (실제 경로에 맞게 변경하세요)
		// 예를 들어, 프로젝트 루트에 serviceAccountKey.json을 둔 경우
		keyPath = "firebase-service-account-key.json"
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			// 파일이 없으면 더 구체적인 에러 메시지를 출력하고 종료
			log.Fatalf("환경 변수 FIREBASE_SERVICE_ACCOUNT_KEY_PATH가 설정되지 않았고, 기본 파일 %s도 찾을 수 없습니다. Firebase 서비스 계정 키 파일의 경로를 정확히 설정해주세요.", keyPath)
		}
	}

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
