package api

import (
	"errors"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Payment struct{}

// Notify 支付回调(免鉴权)
// @Tags Payment
// @Summary 支付异步回调
// @Description EasyPay支付成功后的异步通知
// @Accept  x-www-form-urlencoded
// @Produce  plain
// @Param pid query string true "商户ID"
// @Param trade_no query string true "平台订单号"
// @Param out_trade_no query string true "业务订单号"
// @Param type query string true "支付类型"
// @Param name query string true "订单标题"
// @Param money query string true "金额"
// @Param trade_status query string true "交易状态"
// @Param sign query string true "签名"
// @Param sign_type query string false "签名类型"
// @Success 200 {string} string "success"
// @Failure 400 {string} string "fail"
// @Router /api/payment/notify [get]
func (p *Payment) Notify(c *gin.Context) {
	// 检查支付功能是否启用
	if !service.AllService.PaymentService.IsEnabled() {
		c.String(200, "fail")
		return
	}

	// 收集所有参数(支持GET和POST)
	c.Request.ParseForm()
	params := make(map[string]string)
	for key, values := range c.Request.Form {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	// 处理回调
	err := service.AllService.SubscriptionService.HandleNotify(params)
	if err != nil {
		c.String(200, "fail")
		return
	}

	// 返回成功(必须返回"success"字符串)
	c.String(200, "success")
}

// Submit 支付中转页(免鉴权)
// @Tags Payment
// @Summary 支付中转页
// @Description 生成自动提交表单，以 POST 方式提交到 EasyPay 网关
// @Produce  html
// @Param out_trade_no query string true "业务订单号"
// @Success 200 {string} string "HTML"
// @Router /api/payment/submit [get]
func (p *Payment) Submit(c *gin.Context) {
	if !service.AllService.PaymentService.IsEnabled() {
		c.String(200, "支付未启用")
		return
	}

	outTradeNo := strings.TrimSpace(c.Query("out_trade_no"))
	if outTradeNo == "" {
		c.String(400, "缺少 out_trade_no")
		return
	}

	// 防止连点/重复打开导致重复提交到网关（部分网关会因同 out_trade_no 重复建单报唯一约束冲突）
	const (
		submitDebounceSeconds = int64(3)
		// 超过该时长的待支付订单视为“过期”，自动关闭并重新生成订单号再发起支付
		pendingOrderStaleAfter = 30 * time.Minute
	)

	var order *model.Order
	var blocked bool

	err := service.DB.Transaction(func(tx *gorm.DB) error {
		cur := &model.Order{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("out_trade_no = ?", outTradeNo).
			First(cur).Error; err != nil {
			return err
		}

		// 订单不存在/状态不正确/金额不合法：不做任何副作用
		if cur.Id == 0 || cur.Status != model.OrderStatusPending || cur.Amount <= 0 || strings.TrimSpace(cur.AmountYuan) == "" {
			order = cur
			return nil
		}

		now := time.Now().Unix()
		if cur.PaySubmitAt > 0 && now-cur.PaySubmitAt < submitDebounceSeconds {
			blocked = true
			order = cur
			return nil
		}

		createdAt := time.Time(cur.CreatedAt)
		isStale := !createdAt.IsZero() && time.Since(createdAt) > pendingOrderStaleAfter

		// 已发起过支付或订单过期：关闭旧订单并生成新订单号，避免网关侧重复建单
		if cur.PaySubmitAt > 0 || isStale {
			if err := tx.Model(&model.Order{}).
				Where("user_id = ? AND plan_id = ? AND status = ?", cur.UserId, cur.PlanId, model.OrderStatusPending).
				Update("status", model.OrderStatusClosed).Error; err != nil {
				return err
			}

			newOutTradeNo := service.AllService.SubscriptionService.GenerateOutTradeNo(cur.UserId)
			newOrder := &model.Order{
				UserId:      cur.UserId,
				PlanId:      cur.PlanId,
				OutTradeNo:  newOutTradeNo,
				Subject:     cur.Subject,
				Amount:      cur.Amount,
				AmountYuan:  cur.AmountYuan,
				Status:      model.OrderStatusPending,
				PaySubmitAt: now,
			}
			if err := tx.Create(newOrder).Error; err != nil {
				return err
			}
			order = newOrder
			return nil
		}

		// 首次发起支付：记录发起时间用于防抖/重试判断
		if err := tx.Model(cur).Update("pay_submit_at", now).Error; err != nil {
			return err
		}
		cur.PaySubmitAt = now
		order = cur
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(404, "订单不存在")
			return
		}
		c.String(500, "系统错误")
		return
	}
	if order == nil || order.Id == 0 {
		c.String(404, "订单不存在")
		return
	}
	if order.Status != model.OrderStatusPending {
		c.String(200, "订单状态不可支付")
		return
	}
	if order.Amount <= 0 || strings.TrimSpace(order.AmountYuan) == "" {
		c.String(200, "订单金额不合法")
		return
	}
	if blocked {
		c.String(200, "正在跳转，请勿重复提交")
		return
	}

	action := service.AllService.PaymentService.PaySubmitURL()
	params := service.AllService.PaymentService.BuildPayParams(order.OutTradeNo, order.Subject, order.AmountYuan)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(200, buildAutoSubmitHTML(action, params))
}

func buildAutoSubmitHTML(action string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>正在跳转到支付...</title></head><body>")
	b.WriteString("<form id=\"pay-form\" method=\"post\" action=\"")
	b.WriteString(html.EscapeString(action))
	b.WriteString("\">")
	for _, k := range keys {
		b.WriteString("<input type=\"hidden\" name=\"")
		b.WriteString(html.EscapeString(k))
		b.WriteString("\" value=\"")
		b.WriteString(html.EscapeString(params[k]))
		b.WriteString("\">")
	}
	b.WriteString("</form>")
	b.WriteString("<noscript>请启用 JavaScript 后继续。<button type=\"submit\" form=\"pay-form\">继续</button></noscript>")
	b.WriteString("<script>document.getElementById('pay-form').submit();</script>")
	b.WriteString("</body></html>")
	return b.String()
}

// Plans 获取套餐列表
// @Tags Payment
// @Summary 获取可用套餐列表
// @Description 获取所有启用的订阅套餐
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Router /api/subscription/plans [get]
func (p *Payment) Plans(c *gin.Context) {
	if !service.AllService.PaymentService.IsEnabled() {
		response.Fail(c, 101, response.TranslateMsg(c, "PaymentDisabled"))
		return
	}

	plans := service.AllService.SubscriptionService.ListActivePlans()
	response.Success(c, plans)
}

// CreateOrder 创建订单
// @Tags Payment
// @Summary 创建支付订单
// @Description 创建订单并返回支付跳转URL
// @Accept  json
// @Produce  json
// @Param body body CreateOrderRequest true "创建订单请求"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.ErrorResponse
// @Router /api/subscription/orders [post]
func (p *Payment) CreateOrder(c *gin.Context) {
	if !service.AllService.PaymentService.IsEnabled() {
		response.Fail(c, 101, response.TranslateMsg(c, "PaymentDisabled"))
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	// 获取当前用户
	user := service.AllService.UserService.CurUser(c)
	if user == nil {
		response.Error(c, response.TranslateMsg(c, "UserNotFound"))
		return
	}

	// 创建订单
	outTradeNo, payURL, err := service.AllService.SubscriptionService.CreateOrder(user.Id, req.PlanId)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, err.Error()))
		return
	}

	response.Success(c, gin.H{
		"out_trade_no": outTradeNo,
		"pay_url":      payURL,
	})
}

// Status 获取订阅状态
// @Tags Payment
// @Summary 获取当前用户订阅状态
// @Description 获取当前登录用户的订阅信息
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Router /api/subscription/status [get]
func (p *Payment) Status(c *gin.Context) {
	// 获取当前用户
	user := service.AllService.UserService.CurUser(c)
	if user == nil {
		response.Error(c, response.TranslateMsg(c, "UserNotFound"))
		return
	}

	// 获取订阅信息
	sub := service.AllService.SubscriptionService.GetUserSubscription(user.Id)
	active := service.AllService.SubscriptionService.IsSubscriptionActive(user.Id)

	// 检查支付功能是否启用
	paymentEnabled := service.AllService.PaymentService.IsEnabled()

	response.Success(c, gin.H{
		"payment_enabled": paymentEnabled,
		"active":          active,
		"subscription":    sub,
	})
}

// Orders 获取用户订单列表
// @Tags Payment
// @Summary 获取当前用户订单列表
// @Description 获取当前登录用户的订单历史
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} response.Response
// @Router /api/subscription/orders [get]
func (p *Payment) Orders(c *gin.Context) {
	// 获取当前用户
	user := service.AllService.UserService.CurUser(c)
	if user == nil {
		response.Error(c, response.TranslateMsg(c, "UserNotFound"))
		return
	}

	var req PageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		req.Page = 1
		req.PageSize = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	orders := service.AllService.SubscriptionService.ListOrders(uint(req.Page), uint(req.PageSize), func(tx *gorm.DB) {
		tx.Where("user_id = ?", user.Id)
		if req.Status != nil {
			tx.Where("status = ?", *req.Status)
		}
	})
	// 仅对待支付订单补充 pay_url，便于前端“立即支付”直接跳转，避免重复创建订单
	if service.AllService.PaymentService.IsEnabled() {
		for _, order := range orders.Orders {
			if order == nil {
				continue
			}
			if order.Status == model.OrderStatusPending && order.Amount > 0 {
				order.PayURL = service.AllService.PaymentService.BuildPayURL(order.OutTradeNo)
			}
		}
	}
	response.Success(c, orders)
}

// Request/Response 结构体
type CreateOrderRequest struct {
	PlanId uint `json:"plan_id" binding:"required,gt=0"`
}

type PageRequest struct {
	Page     int  `form:"page" json:"page"`
	PageSize int  `form:"page_size" json:"page_size"`
	Status   *int `form:"status" json:"status"`
}
