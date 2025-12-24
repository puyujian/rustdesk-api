package admin

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

type Payment struct{}

// ========== 套餐管理 ==========

// PlanList 套餐列表
// @Tags Admin-Payment
// @Summary 获取套餐列表
// @Description 获取所有订阅套餐(分页)
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription_plan/list [get]
func (p *Payment) PlanList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	plans := service.AllService.SubscriptionService.ListPlans(uint(page), uint(pageSize), nil)
	response.Success(c, plans)
}

// PlanDetail 套餐详情
// @Tags Admin-Payment
// @Summary 获取套餐详情
// @Description 根据ID获取套餐详情
// @Accept  json
// @Produce  json
// @Param id path int true "套餐ID"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription_plan/detail/{id} [get]
func (p *Payment) PlanDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	plan := service.AllService.SubscriptionService.GetPlanById(uint(id))
	if plan.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "PlanNotFound"))
		return
	}
	response.Success(c, plan)
}

// PlanCreate 创建套餐
// @Tags Admin-Payment
// @Summary 创建套餐
// @Description 创建新的订阅套餐
// @Accept  json
// @Produce  json
// @Param body body PlanForm true "套餐信息"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription_plan/create [post]
func (p *Payment) PlanCreate(c *gin.Context) {
	var form PlanForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	// 验证
	errList := global.Validator.ValidStruct(c, &form)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}

	// 检查编码是否重复
	existing := service.AllService.SubscriptionService.GetPlanByCode(form.Code)
	if existing.Id != 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "PlanCodeExists"))
		return
	}

	plan := &model.SubscriptionPlan{
		Code:        form.Code,
		Name:        form.Name,
		Description: form.Description,
		Price:       form.Price,
		PeriodUnit:  form.PeriodUnit,
		PeriodCount: form.PeriodCount,
		Status:      model.StatusCode(form.Status),
		SortOrder:   form.SortOrder,
	}

	if err := service.AllService.SubscriptionService.CreatePlan(plan); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}

	response.Success(c, plan)
}

// PlanUpdate 更新套餐
// @Tags Admin-Payment
// @Summary 更新套餐
// @Description 更新订阅套餐信息
// @Accept  json
// @Produce  json
// @Param body body PlanForm true "套餐信息"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription_plan/update [post]
func (p *Payment) PlanUpdate(c *gin.Context) {
	var form PlanForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	if form.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	plan := service.AllService.SubscriptionService.GetPlanById(form.Id)
	if plan.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "PlanNotFound"))
		return
	}

	// 检查编码是否重复(排除自身)
	if form.Code != plan.Code {
		existing := service.AllService.SubscriptionService.GetPlanByCode(form.Code)
		if existing.Id != 0 && existing.Id != plan.Id {
			response.Fail(c, 101, response.TranslateMsg(c, "PlanCodeExists"))
			return
		}
	}

	plan.Code = form.Code
	plan.Name = form.Name
	plan.Description = form.Description
	plan.Price = form.Price
	plan.PeriodUnit = form.PeriodUnit
	plan.PeriodCount = form.PeriodCount
	plan.Status = model.StatusCode(form.Status)
	plan.SortOrder = form.SortOrder

	if err := service.AllService.SubscriptionService.UpdatePlan(plan); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}

	response.Success(c, plan)
}

// PlanDelete 删除套餐
// @Tags Admin-Payment
// @Summary 删除套餐
// @Description 删除(禁用)订阅套餐
// @Accept  json
// @Produce  json
// @Param body body IdForm true "套餐ID"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription_plan/delete [post]
func (p *Payment) PlanDelete(c *gin.Context) {
	var form IdForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	if err := service.AllService.SubscriptionService.DeletePlan(form.Id); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}

	response.Success(c, nil)
}

// ========== 订单管理 ==========

// OrderList 订单列表
// @Tags Admin-Payment
// @Summary 获取订单列表
// @Description 获取所有订单(分页)
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param user_id query int false "用户ID"
// @Param status query int false "状态"
// @Param out_trade_no query string false "订单号"
// @Success 200 {object} response.Response
// @Router /api/admin/order/list [get]
func (p *Payment) OrderList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	userId, _ := strconv.Atoi(c.DefaultQuery("user_id", "0"))
	status, _ := strconv.Atoi(c.DefaultQuery("status", "-1"))
	outTradeNo := c.DefaultQuery("out_trade_no", "")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	orders := service.AllService.SubscriptionService.ListOrders(uint(page), uint(pageSize), func(tx *gorm.DB) {
		if userId > 0 {
			tx.Where("user_id = ?", userId)
		}
		if status >= 0 {
			tx.Where("status = ?", status)
		}
		if outTradeNo != "" {
			tx.Where("out_trade_no LIKE ?", "%"+outTradeNo+"%")
		}
	})
	response.Success(c, orders)
}

// OrderDetail 订单详情
// @Tags Admin-Payment
// @Summary 获取订单详情
// @Description 根据ID获取订单详情
// @Accept  json
// @Produce  json
// @Param id path int true "订单ID"
// @Success 200 {object} response.Response
// @Router /api/admin/order/detail/{id} [get]
func (p *Payment) OrderDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	order := service.AllService.SubscriptionService.GetOrderById(uint(id))
	if order.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "OrderNotFound"))
		return
	}
	response.Success(c, order)
}

// OrderRefund 订单退款
// @Tags Admin-Payment
// @Summary 订单退款
// @Description 对已支付订单发起退款
// @Accept  json
// @Produce  json
// @Param body body RefundForm true "退款信息"
// @Success 200 {object} response.Response
// @Router /api/admin/order/refund [post]
func (p *Payment) OrderRefund(c *gin.Context) {
	var form RefundForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	if err := service.AllService.SubscriptionService.RefundOrder(form.OrderId, form.Reason); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, err.Error()))
		return
	}

	response.Success(c, nil)
}

// ========== 订阅管理 ==========

// SubscriptionList 订阅列表
// @Tags Admin-Payment
// @Summary 获取订阅列表
// @Description 获取所有用户订阅(分页)
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param user_id query int false "用户ID"
// @Param status query int false "状态"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription/list [get]
func (p *Payment) SubscriptionList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	userId, _ := strconv.Atoi(c.DefaultQuery("user_id", "0"))
	status, _ := strconv.Atoi(c.DefaultQuery("status", "0"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	subs := service.AllService.SubscriptionService.ListSubscriptions(uint(page), uint(pageSize), func(tx *gorm.DB) {
		if userId > 0 {
			tx.Where("user_id = ?", userId)
		}
		if status > 0 {
			tx.Where("status = ?", status)
		}
	})
	response.Success(c, subs)
}

// SubscriptionDetail 订阅详情
// @Tags Admin-Payment
// @Summary 获取订阅详情
// @Description 根据ID获取订阅详情
// @Accept  json
// @Produce  json
// @Param id path int true "订阅ID"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription/detail/{id} [get]
func (p *Payment) SubscriptionDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	sub := service.AllService.SubscriptionService.GetSubscriptionById(uint(id))
	if sub.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	response.Success(c, sub)
}

// SubscriptionGrant 赠送订阅
// @Tags Admin-Payment
// @Summary 赠送订阅时长
// @Description 管理员为用户赠送订阅时长
// @Accept  json
// @Produce  json
// @Param body body GrantForm true "赠送信息"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription/grant [post]
func (p *Payment) SubscriptionGrant(c *gin.Context) {
	var form GrantForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	if err := service.AllService.SubscriptionService.GrantSubscription(form.UserId, form.PlanId, form.Days); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, err.Error()))
		return
	}

	response.Success(c, nil)
}

// SubscriptionCancel 取消订阅
// @Tags Admin-Payment
// @Summary 取消用户订阅
// @Description 管理员取消用户订阅
// @Accept  json
// @Produce  json
// @Param body body UserIdForm true "用户ID"
// @Success 200 {object} response.Response
// @Router /api/admin/subscription/cancel [post]
func (p *Payment) SubscriptionCancel(c *gin.Context) {
	var form UserIdForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	if err := service.AllService.SubscriptionService.CancelSubscription(form.UserId); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}

	response.Success(c, nil)
}

// ========== 表单结构体 ==========

type PlanForm struct {
	Id          uint   `json:"id"`
	Code        string `json:"code" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	Price       int64  `json:"price" validate:"gte=0"`
	PeriodUnit  string `json:"period_unit" validate:"required,oneof=day month year"`
	PeriodCount int    `json:"period_count" validate:"gt=0"`
	Status      int    `json:"status" validate:"oneof=1 2"`
	SortOrder   int    `json:"sort_order"`
}

type IdForm struct {
	Id uint `json:"id" validate:"required"`
}

type UserIdForm struct {
	UserId uint `json:"user_id" validate:"required"`
}

type RefundForm struct {
	OrderId uint   `json:"order_id" validate:"required"`
	Reason  string `json:"reason"`
}

type GrantForm struct {
	UserId uint `json:"user_id" validate:"required"`
	PlanId uint `json:"plan_id" validate:"required"`
	Days   int  `json:"days" validate:"required,gt=0"`
}

// ========== 支付配置管理 ==========

// PaymentConfigForm 支付配置表单
type PaymentConfigForm struct {
	Enable    bool   `json:"enable"`
	BaseURL   string `json:"base_url"`
	Pid       string `json:"pid"`
	Key       string `json:"key"`
	NotifyURL string `json:"notify_url"`
	ReturnURL string `json:"return_url"`
	Timeout   int    `json:"timeout"`
}

// ConfigGet 获取支付配置
// @Tags Admin-Payment
// @Summary 获取支付配置
// @Description 获取当前支付配置信息
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Router /api/admin/payment/config [get]
func (p *Payment) ConfigGet(c *gin.Context) {
	cfg := service.AllService.PaymentService.GetConfig()
	// 隐藏敏感信息的部分字符
	maskedCfg := &model.PaymentConfig{
		Enable:    cfg.Enable,
		BaseURL:   cfg.BaseURL,
		Pid:       maskString(cfg.Pid),
		Key:       maskString(cfg.Key),
		NotifyURL: cfg.NotifyURL,
		ReturnURL: cfg.ReturnURL,
		Timeout:   cfg.Timeout,
	}
	response.Success(c, maskedCfg)
}

// ConfigGetFull 获取完整支付配置（包含敏感信息）
// @Tags Admin-Payment
// @Summary 获取完整支付配置
// @Description 获取完整支付配置信息（包含密钥）
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Router /api/admin/payment/config/full [get]
func (p *Payment) ConfigGetFull(c *gin.Context) {
	cfg := service.AllService.PaymentService.GetConfig()
	response.Success(c, cfg)
}

// ConfigSave 保存支付配置
// @Tags Admin-Payment
// @Summary 保存支付配置
// @Description 保存支付配置信息
// @Accept  json
// @Produce  json
// @Param body body PaymentConfigForm true "支付配置"
// @Success 200 {object} response.Response
// @Router /api/admin/payment/config [post]
func (p *Payment) ConfigSave(c *gin.Context) {
	var form PaymentConfigForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	// 避免前端拿到脱敏后的 pid/key 直接保存，导致覆盖真实密钥
	current := service.AllService.PaymentService.GetConfig()
	pid := strings.TrimSpace(form.Pid)
	key := strings.TrimSpace(form.Key)
	if pid == "" || pid == maskString(current.Pid) || strings.Contains(pid, "*") {
		pid = current.Pid
	}
	if key == "" || key == maskString(current.Key) || strings.Contains(key, "*") {
		key = current.Key
	}

	cfg := &model.PaymentConfig{
		Enable:    form.Enable,
		BaseURL:   form.BaseURL,
		Pid:       pid,
		Key:       key,
		NotifyURL: form.NotifyURL,
		ReturnURL: form.ReturnURL,
		Timeout:   form.Timeout,
	}

	if err := service.AllService.SystemSettingService.SetPaymentConfig(cfg); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}

	response.Success(c, nil)
}

// maskString 遮蔽字符串中间部分
func maskString(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
