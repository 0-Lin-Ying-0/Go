package infrastructure

import (
	"auto-query-bot/internal/domain"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type HuaweiDeviceRepo struct {
	Client *http.Client
}

func NewHuaweiDeviceRepo(client *http.Client) *HuaweiDeviceRepo {
	return &HuaweiDeviceRepo{Client: client}
}

func (r *HuaweiDeviceRepo) Find(sn string, code string) (*domain.DeviceInfo, error) {
	// 真实的查询接口 (GET请求)
	baseURL := "https://app.huawei.com/escpportal/services/portal/vyborgTask/findHardWareVyborgForWeb"

	params := url.Values{}
	params.Set("barcode", sn)
	params.Set("paramCode", code)
	params.Set("buType", "1")
	params.Set("source", "escp")
	params.Set("language", "cn")
	params.Set("_", fmt.Sprintf("%d", time.Now().UnixMilli()))

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, _ := http.NewRequest("GET", fullURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// 发送请求 (带着之前的 Cookie)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 解析 HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// 获取网页纯文本，用于判断是否成功
	// 注意：之后你需要根据 F12 的 HTML 结构，用 doc.Find("#id").Text() 来精准提取数据
	fullText := doc.Text()

	// 去除多余的空行，只保留紧凑的文本
	cleanText := strings.Join(strings.Fields(fullText), " ")

	return &domain.DeviceInfo{
		SN:      sn,
		RawHTML: cleanText, // 这里暂时存所有文本，等你有了真实数据再做精准提取
		Status:  "Done",
	}, nil
}
