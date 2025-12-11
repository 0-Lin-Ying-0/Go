package main

import (
	"auto-query-bot/internal/infrastructure"
	"log"
	"net/http"
	"net/http/cookiejar"
	"time"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	captchaRepo := infrastructure.NewHuaweiCaptchaRepo(client)

	captcha, err := captchaRepo.Fetch()
	if err != nil {
		log.Fatalf("Fetch 失败: %v", err)
	}

	// 调一次 Solve，看能不能生成 manual_captcha.jpg 并读到你手动输入的验证码
	code, err := captchaRepo.Solve(captcha)
	if err != nil {
		log.Fatalf("Solve 失败: %v", err)
	}

	log.Printf("你输入的验证码是: %s\n", code)
}
