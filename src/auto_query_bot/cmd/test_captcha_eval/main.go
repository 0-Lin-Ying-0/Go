package main

import (
	"auto-query-bot/internal/infrastructure"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"time"
)

type sampleResult struct {
	Index    int
	RawPath  string
	ProcPath string
	OCRCode  string
}

func main() {
	const sampleCount = 50                  // 想抓多少张验证码，这里先写 50
	outDir := "captchas"                    // 所有样本都放在这个目录下
	tempProcessed := "temp_ocr_process.png" // Solve 里用的临时文件名

	// 1. 确保输出目录存在
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("创建目录 %s 失败: %v", outDir, err)
	}

	// 2. 初始化 HTTP 客户端 + 仓库
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
	}
	captchaRepo := infrastructure.NewHuaweiCaptchaRepo(client)

	// 3. 开始采样
	results := make([]sampleResult, 0, sampleCount)

	for i := 1; i <= sampleCount; i++ {
		log.Printf("==== 第 %d/%d 个样本 ====", i, sampleCount)

		// 3.1 拉一张验证码
		captcha, err := captchaRepo.Fetch()
		if err != nil {
			log.Fatalf("第 %d 个样本 Fetch 失败: %v", i, err)
		}

		// 3.2 保存原始图片
		rawFile := filepath.Join(outDir, fmt.Sprintf("raw_%03d.png", i))
		if err := os.WriteFile(rawFile, captcha.ImageBytes, 0644); err != nil {
			log.Fatalf("保存原始图片失败(样本 %d): %v", i, err)
		}

		// 3.3 调用现有 Solve 做预处理 + OCR
		code, err := captchaRepo.Solve(captcha)
		if err != nil {
			log.Printf("样本 %d Solve 失败: %v", i, err)
			code = ""
		}

		// 3.4 把 Solve 生成的 temp_ocr_process.png 复制一份，留作该样本的“预处理图”
		procFile := filepath.Join(outDir, fmt.Sprintf("proc_%03d.png", i))

		if data, err := os.ReadFile(tempProcessed); err != nil {
			log.Printf("样本 %d: 读取处理后图片失败: %v", i, err)
		} else if err := os.WriteFile(procFile, data, 0644); err != nil {
			log.Printf("样本 %d: 保存处理后图片失败: %v", i, err)
		}

		log.Printf("样本 %d: OCR 结果 = [%s], 原图 = %s, 预处理图 = %s",
			i, code, rawFile, procFile)

		results = append(results, sampleResult{
			Index:    i,
			RawPath:  rawFile,
			ProcPath: procFile,
			OCRCode:  code,
		})

		// 稍微歇一会儿，别把华为打懵了
		time.Sleep(500 * time.Millisecond)
	}

	// 4. 把所有样本元数据写成一个 CSV，方便你后面标注真实值
	csvFile := filepath.Join(outDir, "ocr_samples.csv")
	file, err := os.Create(csvFile)
	if err != nil {
		log.Fatalf("创建 CSV 文件失败: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 表头: real_code 先留空，等你后面手动填
	if err := writer.Write([]string{"index", "raw_path", "processed_path", "ocr_code", "real_code"}); err != nil {
		log.Fatalf("写 CSV 表头失败: %v", err)
	}

	for _, r := range results {
		record := []string{
			fmt.Sprintf("%d", r.Index),
			r.RawPath,
			r.ProcPath,
			r.OCRCode,
			"", // real_code 留空，等你后面人工填
		}
		if err := writer.Write(record); err != nil {
			log.Printf("写 CSV 行失败(样本 %d): %v", r.Index, err)
		}
	}

	log.Printf("\n采样完成，图片和 CSV 已保存到目录: %s", outDir)
	log.Printf("请用 Excel 打开 %s，查看每一行的图片，手动填写 real_code 列为“真实验证码”。", csvFile)
}
