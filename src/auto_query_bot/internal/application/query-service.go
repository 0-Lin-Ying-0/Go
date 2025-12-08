package service

import (
	"auto-query-bot/internal/domain"
	"fmt"
	"log"
	"strings"
)

type AutoQueryService struct {
	captchaRepo domain.CaptchaRepository
	deviceRepo  domain.DeviceRepository
}

func NewAutoQueryService(cRepo domain.CaptchaRepository, dRepo domain.DeviceRepository) *AutoQueryService {
	return &AutoQueryService{
		captchaRepo: cRepo,
		deviceRepo:  dRepo,
	}
}

func (s *AutoQueryService) Execute(sn string) (*domain.DeviceInfo, error) {
	// 1. 获取验证码
	captcha, err := s.captchaRepo.Fetch()
	if err != nil {
		return nil, fmt.Errorf("下载验证码失败: %w", err)
	}

	// 2. 识别验证码
	code, err := s.captchaRepo.Solve(captcha)
	if err != nil {
		return nil, fmt.Errorf("OCR识别失败: %w", err)
	}
	// 如果识别为空，大概率失败，但也尝试提交一下
	if code == "" {
		log.Println("警告: OCR 识别结果为空，可能需要优化图像处理参数")
	}

	// 3. 执行查询
	info, err := s.deviceRepo.Find(sn, code)
	if err != nil {
		return nil, fmt.Errorf("查询请求失败: %w", err)
	}

	// 简单的业务判断
	if strings.Contains(info.RawHTML, "验证码不正确") || strings.Contains(info.RawHTML, "验证码错误") {
		return nil, fmt.Errorf("验证码错误 (OCR读出的码: %s)", code)
	}

	return info, nil
}
