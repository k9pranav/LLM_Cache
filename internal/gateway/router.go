package gateway

import (
	"github.com/gin-gonic/gin"
)

func NewRouter(
	h *Handler,
	healthPath string,
	metricsPath string,
) *gin.Engine {
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(LoggingMiddleware())
	r.Use(MetricsMiddleware(h.Stats))

	if healthPath == "" {
		healthPath = "/healthz"
	}

	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	r.GET(healthPath, h.Health)
	r.GET("/health", h.Health)

	r.POST("/v1/chat/completions", h.ChatCompletion)

	r.GET("/admin/stats", h.CacheStats)
	r.GET(metricsPath, h.Metrics)

	return r
}
