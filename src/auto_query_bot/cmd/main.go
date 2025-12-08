package main

import (
	service "auto-query-bot/internal/application"
	"auto-query-bot/internal/infrastructure"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"time"
)

func main() {
	fmt.Println(">>>  华为设备批量查询机器人启动...")

	// 1. [Infrastructure] 初始化
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	captchaRepo := infrastructure.NewHuaweiCaptchaRepo(client)
	deviceRepo := infrastructure.NewHuaweiDeviceRepo(client)
	fileHandler := infrastructure.NewFileHandler()

	// 2. [Service] 组装
	queryService := service.NewAutoQueryService(captchaRepo, deviceRepo)

	// 3. 准备文件
	inputFile := "input.txt"
	outputFile := "results.csv"

	// 写入表头
	_ = fileHandler.WriteRecord(outputFile, []string{"序列号(SN)", "验证码", "状态", "原始信息", "时间"})

	// 读取输入
	snList, err := fileHandler.ReadLines(inputFile)
	if err != nil {
		log.Fatalf(" 无法读取 input.txt，请检查文件是否存在: %v", err)
	}
	fmt.Printf(">>>  任务加载完成: 共 %d 条序列号\n", len(snList))

	// 4. 批量执行
	for i, sn := range snList {
		fmt.Printf("\n>>> [%d/%d] 处理 SN: %s\n", i+1, len(snList), sn)

		// 核心调用
		info, err := queryService.Execute(sn)

		timestamp := time.Now().Format("2006-01-02 15:04:05")
		var record []string

		if err != nil {
			fmt.Printf(" 失败: %v\n", err)
			record = []string{sn, "N/A", "Fail", fmt.Sprintf("%v", err), timestamp}
		} else {
			// 只截取前100个字符显示在控制台，防止刷屏
			displayInfo := info.RawHTML
			if len(displayInfo) > 100 {
				displayInfo = displayInfo[:100] + "..."
			}
			fmt.Printf(" 响应: %s\n", displayInfo)
			record = []string{sn, "OCR自动填", "Success", info.RawHTML, timestamp}
		}

		// 写入结果
		_ = fileHandler.WriteRecord(outputFile, record)

		// 风控休眠 (3-6秒随机)
		sleepMs := 3000 + rand.Intn(3000)
		fmt.Printf(" 休眠 %.1f 秒...\n", float64(sleepMs)/1000)
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	}

	fmt.Println("\n>>> 所有任务结束！结果已保存至 results.csv")
}
