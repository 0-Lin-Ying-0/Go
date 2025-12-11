package infrastructure

import (
	"auto-query-bot/internal/domain"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/disintegration/imaging"
)

type HuaweiCaptchaRepo struct {
	Client  *http.Client
	BaseURL string
}

func NewHuaweiCaptchaRepo(client *http.Client) *HuaweiCaptchaRepo {
	return &HuaweiCaptchaRepo{
		Client:  client,
		BaseURL: "https://app.huawei.com/escpportal/servlet/captcha",
	}
}

// Fetch 下载验证码
func (r *HuaweiCaptchaRepo) Fetch() (*domain.Captcha, error) {
	// 加时间戳防止缓存
	url := fmt.Sprintf("%s?yzm=%d", r.BaseURL, time.Now().UnixMilli())
	fmt.Println("captcha url =", url)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://app.huawei.com/escpportal/pub/wechat.html?Language=CN")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("下载失败 HTTP %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &domain.Captcha{ImageBytes: imgData}, nil
}

// Solve 识别验证码
func (r *HuaweiCaptchaRepo) Solve(c *domain.Captcha) (string, error) {
	//// TODO: 先不接 Tesseract，直接让用户手动输入
	//
	fmt.Printf("已获取验证码图片，长度: %d 字节\n", len(c.ImageBytes))
	//fmt.Println("暂不使用 OCR，请手动在浏览器打开图片并输入验证码：")

	// 把图片写成文件，手动打开
	if err := os.WriteFile("manual_captcha.jpg", c.ImageBytes, 0644); err != nil {
		return "", fmt.Errorf("写验证码图片失败: %w", err)
	}
	//
	//fmt.Println("图片已保存为 manual_captcha.jpg，请打开并输入验证码：")
	//
	//var code string
	//fmt.Print("验证码 = ")
	//fmt.Scanln(&code)
	//
	//return code, nil

	// 1. 内存解码
	img, err := imaging.Decode(bytes.NewReader(c.ImageBytes))
	if err != nil {
		return "", err
	}

	// 2. 图像预处理 (去噪)
	img = imaging.Resize(img, 0, 150, imaging.Lanczos) // 放大
	//inverted := imaging.Invert(img)                       // 反转颜色
	gray := imaging.Grayscale(img)   // 灰度化
	Blur := imaging.Blur(gray, 0.8)  // 高斯模糊
	processed := binarize(Blur, 180) // 简单二值化 threshold 可以微调 160~200
	//processed := imaging.AdjustContrast(Blur, 60)   // 提高对比度
	//processed = imaging.AdjustGamma(processed, 0.5) // 降低Gamma值(变暗)，让浅色字显出来

	// 3. 临时保存给 OCR 读取
	tempFile := "temp_ocr_process.png"

	if err := imaging.Save(processed, tempFile); err != nil {
		return "", fmt.Errorf("图片保存失败：%v", err)
	}
	//defer os.Remove(tempFile) // 用完删除

	//// 3. OCR 识别
	//client := gosseract.NewClient()
	//defer client.Close()

	// 只识别数字
	//client.SetWhitelist("0123456789")
	//client.SetImage(tempFile)
	//
	//text, err := client.Text()
	//return text, err

	// 4. 调用系统安装的 Tesseract
	// 只要 CMD 里能敲 tesseract，这行代码就能跑
	cmd := exec.Command("tesseract", tempFile, "stdout",
		"--psm", "7",
		"--oem", "1", // LSTM OCR Engine
		"-c", "tessedit_char_whitelist=0123456789",
		"-c", "classify_bln_numeric_mode=1",
	)

	var out bytes.Buffer
	cmd.Stdout = &out

	// 运行命令
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("OCR执行错误(请检查是否安装Tesseract): %v", err)
	}

	// 5. 清洗结果
	result := strings.TrimSpace(out.String())
	// 只保留 0-9
	digits := make([]int32, 0, len(result))
	for _, r := range result {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}

	code := string(digits)

	//如果识别出了超过/低于 4 位，
	if len(code) != 4 {
		return "", fmt.Errorf("验证码长度错误: 期望 4 位，实际识别为 %d 位", len(code))
	}
	return code, nil
}

func binarize(src image.Image, threshold uint8) *image.Gray {
	b := src.Bounds()
	dst := image.NewGray(b)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.GrayModel.Convert(src.At(x, y)).(color.Gray)
			if c.Y > threshold {
				dst.SetGray(x, y, color.Gray{Y: 255}) // 亮的当背景(白)
			} else {
				dst.SetGray(x, y, color.Gray{Y: 0}) // 暗的当前景(黑)
			}
		}
	}
	return dst
}
