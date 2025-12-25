package middleware

import (
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// InternalAuth 内部接口鉴权中间件
// 用于保护 /api/internal/* 接口
//
// 安全策略:
// 1. 如果配置了 RUSTDESK_API_INTERNAL_KEY，则必须携带正确的 X-Internal-Key 头
// 2. 如果未配置密钥，则仅允许本地回环地址访问 (127.0.0.1/::1)
// 3. 内网 IP 不再自动放行，必须配合密钥使用
func InternalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		internalKey := os.Getenv("RUSTDESK_API_INTERNAL_KEY")

		// 获取真实客户端 IP (使用 RemoteAddr，不信任代理头)
		clientIP := getRemoteIP(c)

		// 情况1: 配置了内部密钥
		if internalKey != "" {
			headerKey := c.GetHeader("X-Internal-Key")
			if headerKey == internalKey {
				// 密钥正确，放行
				c.Next()
				return
			}
			// 密钥错误或未提供，拒绝
			c.JSON(403, gin.H{
				"code":  403,
				"error": "Forbidden: invalid or missing X-Internal-Key",
			})
			c.Abort()
			return
		}

		// 情况2: 未配置密钥，仅允许本地回环地址
		if isLoopback(clientIP) {
			c.Next()
			return
		}

		// 拒绝访问
		c.JSON(403, gin.H{
			"code":  403,
			"error": "Forbidden: internal API requires X-Internal-Key or localhost access",
		})
		c.Abort()
	}
}

// getRemoteIP 获取真实客户端 IP (不信任代理头)
func getRemoteIP(c *gin.Context) string {
	// 直接从 RemoteAddr 获取，格式为 "ip:port"
	remoteAddr := c.Request.RemoteAddr
	if remoteAddr == "" {
		return ""
	}

	// 分离 IP 和端口
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// 可能没有端口，直接返回
		return remoteAddr
	}
	return host
}

// isLoopback 检查是否为本地回环地址
func isLoopback(ipStr string) bool {
	if ipStr == "" {
		return false
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		// 可能是 localhost
		return strings.HasPrefix(ipStr, "127.") || ipStr == "::1" || ipStr == "localhost"
	}
	return ip.IsLoopback()
}

// isPrivateIP 检查是否为私有 IP 地址 (保留但不再用于鉴权)
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// 私有 IP 范围
	privateRanges := []string{
		"10.0.0.0/8",     // Class A
		"172.16.0.0/12",  // Class B
		"192.168.0.0/16", // Class C
		"fc00::/7",       // IPv6 ULA
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}
