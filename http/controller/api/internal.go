package api

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Internal 内部接口控制器
// 供 hbbs/hbbr 调用，需要内部鉴权
type Internal struct{}

// 安全限制常量
const (
	MaxUUIDLength = 128  // UUID 最大长度
	MaxSlots      = 10   // 最大 slots 数
	MaxTTLSec     = 300  // 最大 TTL (秒)
	MaxTokenLen   = 2048 // Token 最大长度
)

// RelayAllowRequest relay 白名单写入请求
type RelayAllowRequest struct {
	UUID   string `json:"uuid" binding:"required"`
	Slots  int    `json:"slots"`   // 默认 2，最大 10
	TTLSec int    `json:"ttl_sec"` // 默认 120，最大 300
}

// RelayConsumeRequest relay 白名单消费请求
type RelayConsumeRequest struct {
	UUID string `json:"uuid" binding:"required"`
}

// SubscriptionCheckRequest 订阅检查请求 (支持 POST body)
type SubscriptionCheckRequest struct {
	Token string `json:"token"`
	UUID  string `json:"uuid"`
}

// RelayAllow 写入 relay 白名单
// @Tags Internal
// @Summary 写入 relay 白名单
// @Description hbbs 调用，允许指定 uuid 进行 relay 连接
// @Accept json
// @Produce json
// @Param request body RelayAllowRequest true "请求参数"
// @Success 200 {object} response.Response
// @Router /api/internal/relay/allow [post]
func (i *Internal) RelayAllow(c *gin.Context) {
	var req RelayAllowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, "invalid request: "+err.Error())
		return
	}

	if req.UUID == "" {
		response.Fail(c, 400, "uuid is required")
		return
	}

	// 安全检查: UUID 长度限制
	if len(req.UUID) > MaxUUIDLength {
		response.Fail(c, 400, "uuid too long")
		return
	}

	// 默认值和上限限制
	if req.Slots <= 0 {
		req.Slots = 2
	} else if req.Slots > MaxSlots {
		req.Slots = MaxSlots
	}

	if req.TTLSec <= 0 {
		req.TTLSec = 120
	} else if req.TTLSec > MaxTTLSec {
		req.TTLSec = MaxTTLSec
	}

	service.AllService.RelayWhitelistService.Allow(req.UUID, req.Slots, req.TTLSec)

	response.Success(c, gin.H{
		"uuid":    req.UUID,
		"slots":   req.Slots,
		"ttl_sec": req.TTLSec,
	})
}

// RelayConsume 消费 relay 白名单
// @Tags Internal
// @Summary 消费 relay 白名单
// @Description hbbr 调用，验证并消费指定 uuid 的白名单额度
// @Accept json
// @Produce json
// @Param request body RelayConsumeRequest true "请求参数"
// @Success 200 {object} response.Response
// @Router /api/internal/relay/consume [post]
func (i *Internal) RelayConsume(c *gin.Context) {
	var req RelayConsumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, "invalid request: "+err.Error())
		return
	}

	if req.UUID == "" {
		response.Fail(c, 400, "uuid is required")
		return
	}

	// 安全检查: UUID 长度限制
	if len(req.UUID) > MaxUUIDLength {
		response.Fail(c, 400, "uuid too long")
		return
	}

	allowed := service.AllService.RelayWhitelistService.Consume(req.UUID)

	response.Success(c, gin.H{
		"uuid":    req.UUID,
		"allowed": allowed,
	})
}

// SubscriptionCheck 订阅状态检查
// @Tags Internal
// @Summary 内部订阅状态检查
// @Description 通过 token 或 uuid 检查订阅状态，供 hbbs/hbbr 调用。推荐使用 POST body 传递 token 以避免日志泄露
// @Accept json
// @Produce json
// @Param request body SubscriptionCheckRequest false "请求参数 (POST body)"
// @Success 200 {object} response.Response
// @Router /api/internal/subscription/check [post]
func (i *Internal) SubscriptionCheck(c *gin.Context) {
	var token, uuid string

	// 优先从 POST body 获取 (推荐，避免 token 泄露到日志)
	var req SubscriptionCheckRequest
	if err := c.ShouldBindJSON(&req); err == nil {
		token = req.Token
		uuid = req.UUID
	}

	// 向后兼容: 也支持 GET query (不推荐)
	if token == "" {
		token = c.Query("token")
	}
	if uuid == "" {
		uuid = c.Query("uuid")
	}

	// 安全检查: Token 长度限制
	if len(token) > MaxTokenLen {
		response.Fail(c, 400, "token too long")
		return
	}

	var userId uint

	// 优先通过 token 获取 user_id
	if token != "" {
		uid, err := service.Jwt.ParseToken(token)
		if err == nil && uid > 0 {
			userId = uid
		}
	}

	// 如果 token 无效，尝试通过 uuid 获取 user_id
	if userId == 0 && uuid != "" {
		peer := service.AllService.PeerService.FindByUuid(uuid)
		if peer.RowId > 0 {
			userId = peer.UserId
		}
	}

	// 检查支付功能是否启用
	paymentEnabled := service.AllService.PaymentService.IsEnabled()

	// 如果支付未启用，直接放行
	if !paymentEnabled {
		response.Success(c, gin.H{
			"active":          true,
			"payment_enabled": false,
			"reason":          "payment_disabled",
		})
		return
	}

	// 无法识别用户
	if userId == 0 {
		response.Success(c, gin.H{
			"active":          false,
			"payment_enabled": true,
			"reason":          "user_not_found",
		})
		return
	}

	// 检查订阅状态
	active := service.AllService.SubscriptionService.IsSubscriptionActive(userId)

	response.Success(c, gin.H{
		"active":          active,
		"payment_enabled": true,
		"user_id":         userId,
	})
}

// RelayStats 白名单统计信息
// @Tags Internal
// @Summary 白名单统计信息
// @Description 获取当前白名单统计信息
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/internal/relay/stats [get]
func (i *Internal) RelayStats(c *gin.Context) {
	stats := service.AllService.RelayWhitelistService.Stats()
	response.Success(c, stats)
}
