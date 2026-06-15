package gateway

import (
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const ContextTenantIDKey = "tenant_id"
const ContextUserIDKey = "user_id"

func AuthMiddleware(expectedAPIKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := strings.TrimSpace(c.GetHeader("X-Tenant-ID"))
		if tenantID == "" {
			tenantID = "default"
		}

		userID := strings.TrimSpace(c.GetHeader("X-User-ID"))

		c.Set(ContextTenantIDKey, tenantID)
		c.Set(ContextUserIDKey, userID)

		if strings.TrimSpace(expectedAPIKey) == "" {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

		if token == "" || token != expectedAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		log.Printf(
			"%s %s status=%d latency=%s",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			time.Since(start),
		)
	}
}

func MetricsMiddleware(stats *GatewayStats) gin.HandlerFunc {
	return func(c *gin.Context) {
		atomic.AddInt64(&stats.HTTPRequestsTotal, 1)
		c.Next()
	}
}
