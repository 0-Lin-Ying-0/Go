package infrastructure

import (
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

// 通用的底层请求发送器
func (r *HuaweiDeviceRepo) sendRequest(baseURL string, sn string, code string) (string, error) {
	params := url.Values{}
	params.Set("barcode", sn)
	params.Set("paramCode", code)
	params.Set("buType", "1")
	params.Set("source", "escp")
	params.Set("language", "cn")
	params.Set("_", fmt.Sprintf("%d", time.Now().UnixMilli()))

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, _ := http.NewRequest("GET", fullURL, nil)

	// 伪装
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://app.huawei.com/escpportal/pub/wechat.html?Language=CN")

	// 发送请求 (带着之前的 Cookie)
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 解析 HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	// 获取网页纯文本，用于判断是否成功
	// 注意：之后需要根据 F12 的 HTML 结构，用 doc.Find("#id").Text() 来精准提取数据
	fullText := doc.Text()

	// 提取纯文本
	return strings.Join(strings.Fields(fullText), " "), nil
}

func (r *HuaweiDeviceRepo) FindHardware(sn string, code string) (string, error) {
	// 真实的查询接口 (GET请求)
	baseURL := "https://app.huawei.com/escpportal/services/portal/vyborgTask/findHardWareVyborgForWeb"
	return r.sendRequest(baseURL, sn, code)
}

func (r *HuaweiDeviceRepo) FindSoftware(sn string, code string) (string, error) {
	// 真实的查询接口 (GET请求)
	baseURL := "https://app.huawei.com/escpportal/services/portal/vyborgTask/findSoftwareVyborgForWeb"
	return r.sendRequest(baseURL, sn, code)
}
