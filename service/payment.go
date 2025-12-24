package service

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

type PaymentService struct{}

// EasyPay 响应结构
type EpayQueryResp struct {
	Code       int    `json:"code"`
	Msg        string `json:"msg"`
	TradeNo    string `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Type       string `json:"type"`
	Pid        string `json:"pid"`
	AddTime    string `json:"addtime"`
	EndTime    string `json:"endtime"`
	Name       string `json:"name"`
	Money      string `json:"money"`
	Status     int    `json:"status"` // 1=成功 0=失败/处理中
}

type EpayRefundResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// getConfig 获取支付配置（优先从数据库读取）
func (ps *PaymentService) getConfig() *model.PaymentConfig {
	return AllService.SystemSettingService.GetPaymentConfig()
}

// Sign 生成签名
// 按 EasyPay 协议: 非空字段(排除sign/sign_type) -> ASCII升序 -> k1=v1&k2=v2 -> 末尾追加secret -> MD5小写
func (ps *PaymentService) Sign(params map[string]string) string {
	cfg := ps.getConfig()

	// 1. 过滤空值和sign/sign_type
	filtered := make(map[string]string)
	for k, v := range params {
		if v == "" || k == "sign" || k == "sign_type" {
			continue
		}
		filtered[k] = v
	}

	// 2. 按key ASCII升序排序
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 3. 拼接 k=v&k=v
	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, filtered[k]))
	}
	str := strings.Join(pairs, "&")

	// 4. 末尾追加secret
	str += cfg.Key

	// 5. MD5小写
	hash := md5.Sum([]byte(str))
	return hex.EncodeToString(hash[:])
}

// Verify 验证签名(使用常量时间比较防止时序攻击)
func (ps *PaymentService) Verify(params map[string]string) bool {
	got := params["sign"]
	if got == "" {
		return false
	}
	expected := ps.Sign(params)
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(got)), []byte(strings.ToLower(expected))) == 1
}

// PaySubmitURL 获取 EasyPay 提交地址
func (ps *PaymentService) PaySubmitURL() string {
	cfg := ps.getConfig()
	return strings.TrimRight(cfg.BaseURL, "/") + "/pay/submit.php"
}

// BuildPayParams 构建提交到 EasyPay 的表单参数
func (ps *PaymentService) BuildPayParams(outTradeNo, subject, moneyYuan string) map[string]string {
	cfg := ps.getConfig()

	params := map[string]string{
		"pid":          cfg.Pid,
		"type":         "epay",
		"out_trade_no": outTradeNo,
		"name":         subject,
		"money":        moneyYuan,
		"sign_type":    "MD5",
	}
	if cfg.NotifyURL != "" {
		params["notify_url"] = cfg.NotifyURL
	}
	if cfg.ReturnURL != "" {
		params["return_url"] = cfg.ReturnURL
	}

	// 生成签名
	sign := ps.Sign(params)
	params["sign"] = sign

	return params
}

// BuildPayURL 构建支付跳转URL（返回本服务的中转页面，用于以 POST 方式提交到网关）
func (ps *PaymentService) BuildPayURL(outTradeNo string) string {
	q := url.Values{}
	q.Set("out_trade_no", outTradeNo)
	return "/api/payment/submit?" + q.Encode()
}

// Query 查询订单状态
func (ps *PaymentService) Query(outTradeNo string) (*EpayQueryResp, error) {
	cfg := ps.getConfig()

	q := url.Values{}
	q.Set("act", "order")
	q.Set("pid", cfg.Pid)
	q.Set("key", cfg.Key)
	q.Set("out_trade_no", outTradeNo)

	reqURL := cfg.BaseURL + "/api.php?" + q.Encode()

	client := ps.getHTTPClient()
	resp, err := client.Get(reqURL)
	if err != nil {
		Logger.Error("Payment query request failed: ", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Logger.Error("Payment query read body failed: ", err)
		return nil, err
	}

	var result EpayQueryResp
	if err := json.Unmarshal(body, &result); err != nil {
		Logger.Error("Payment query parse response failed: ", err, " body: ", string(body))
		return nil, err
	}

	return &result, nil
}

// Refund 发起退款
func (ps *PaymentService) Refund(tradeNo, moneyYuan string) (*EpayRefundResp, error) {
	cfg := ps.getConfig()

	data := url.Values{}
	data.Set("pid", cfg.Pid)
	data.Set("key", cfg.Key)
	data.Set("trade_no", tradeNo)
	data.Set("money", moneyYuan)

	reqURL := cfg.BaseURL + "/api.php"

	client := ps.getHTTPClient()
	resp, err := client.PostForm(reqURL, data)
	if err != nil {
		Logger.Error("Payment refund request failed: ", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Logger.Error("Payment refund read body failed: ", err)
		return nil, err
	}

	var result EpayRefundResp
	if err := json.Unmarshal(body, &result); err != nil {
		Logger.Error("Payment refund parse response failed: ", err, " body: ", string(body))
		return nil, err
	}

	if result.Code != 1 {
		return &result, errors.New(result.Msg)
	}

	return &result, nil
}

// getHTTPClient 获取HTTP客户端(复用代理配置)
func (ps *PaymentService) getHTTPClient() *http.Client {
	cfg := ps.getConfig()
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	if Config.Proxy.Enable && Config.Proxy.Host != "" {
		proxyURL, err := url.Parse(Config.Proxy.Host)
		if err != nil {
			Logger.Warn("Invalid proxy URL: ", err)
			return &http.Client{Timeout: timeout}
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
		return &http.Client{
			Transport: transport,
			Timeout:   timeout,
		}
	}

	return &http.Client{Timeout: timeout}
}

// IsEnabled 检查支付功能是否启用
func (ps *PaymentService) IsEnabled() bool {
	cfg := ps.getConfig()
	return cfg.Enable
}

// GetConfig 获取支付配置（公开方法，用于API返回）
func (ps *PaymentService) GetConfig() *model.PaymentConfig {
	return ps.getConfig()
}
