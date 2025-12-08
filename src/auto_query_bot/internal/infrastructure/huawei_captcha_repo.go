package infrastructure

import (
	"auto-query-bot/internal/domain"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract/v2"
)

type HuaweiCaptchaRepo struct {
	Client  *http.Client
	BaseURL string
}

func NewHuaweiCaptchaRepo(client *http.Client) *HuaweiCaptchaRepo {
	return &HuaweiCaptchaRepo{
		Client:  client,
		BaseURL: "https://app.huawei.com/escpportal/servlet/captchaValidate",
	}
}

// Fetch 下载验证码
func (r *HuaweiCaptchaRepo) Fetch() (*domain.Captcha, error) {
	// 加时间戳防止缓存
	url := fmt.Sprintf("%s?yzm=%d", r.BaseURL, time.Now().UnixMilli())

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &domain.Captcha{ImageBytes: imgData}, nil
}

// Solve 识别验证码
func (r *HuaweiCaptchaRepo) Solve(c *domain.Captcha) (string, error) {
	// 1. 内存解码
	img, err := imaging.Decode(bytes.NewReader(c.ImageBytes))
	if err != nil {
		return "", err
	}

	// 2. 图像预处理 (去噪)
	gray := imaging.Grayscale(img)
	processed := imaging.AdjustContrast(gray, 20) // 提高对比度

	// 临时保存给 OCR 读取
	tempFile := "temp_ocr_process.png"
	_ = imaging.Save(processed, tempFile)
	defer os.Remove(tempFile) // 用完删除

	// 3. OCR 识别
	client := gosseract.NewClient()
	defer client.Close()

	// 只识别数字
	client.SetWhitelist("0123456789")
	client.SetImage(tempFile)

	text, err := client.Text()
	return text, err
}
