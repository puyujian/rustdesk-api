package api

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
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

	orders := service.AllService.SubscriptionService.ListUserOrders(user.Id, uint(req.Page), uint(req.PageSize))
	response.Success(c, orders)
}

// Request/Response 结构体
type CreateOrderRequest struct {
	PlanId uint `json:"plan_id" binding:"required,gt=0"`
}

type PageRequest struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}
