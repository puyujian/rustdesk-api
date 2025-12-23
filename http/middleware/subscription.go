package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// RequireSubscription 订阅检查中间件
// 必须在 RustAuth() 之后使用
func RequireSubscription() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查支付功能是否启用
		if !service.AllService.PaymentService.IsEnabled() {
			// 支付功能未启用,直接放行
			c.Next()
			return
		}

		// 获取当前用户
		user := service.AllService.UserService.CurUser(c)
		if user == nil {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		// 管理员免检查
		if user.IsAdmin != nil && *user.IsAdmin {
			c.Next()
			return
		}

		// 检查订阅状态
		if !service.AllService.SubscriptionService.IsSubscriptionActive(user.Id) {
			// 返回 402 Payment Required
			response.Fail(c, 402, response.TranslateMsg(c, "SubscriptionRequired"))
			c.Abort()
			return
		}

		c.Next()
	}
}
