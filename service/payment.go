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

// Sign 生成签名
// 按 EasyPay 协议: 非空字段(排除sign/sign_type) -> ASCII升序 -> k1=v1&k2=v2 -> 末尾追加secret -> MD5小写
func (ps *PaymentService) Sign(params map[string]string) string {
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
	str += Config.Payment.EasyPay.Key

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

// BuildPayURL 构建支付跳转URL
func (ps *PaymentService) BuildPayURL(outTradeNo, subject, moneyYuan string) string {
	cfg := Config.Payment.EasyPay

	params := map[string]string{
		"pid":          cfg.Pid,
		"type":         "epay",
		"out_trade_no": outTradeNo,
		"name":         subject,
		"money":        moneyYuan,
		"notify_url":   cfg.NotifyURL,
		"return_url":   cfg.ReturnURL,
		"sign_type":    "MD5",
	}

	// 生成签名
	sign := ps.Sign(params)
	params["sign"] = sign

	// 构建URL
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}

	return cfg.BaseURL + "/pay/submit.php?" + q.Encode()
}

// Query 查询订单状态
func (ps *PaymentService) Query(outTradeNo string) (*EpayQueryResp, error) {
	cfg := Config.Payment.EasyPay

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
	cfg := Config.Payment.EasyPay

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
	timeout := Config.Payment.EasyPay.Timeout
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
	return Config.Payment.EasyPay.Enable
}
