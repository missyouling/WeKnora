package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
)

// ErrorHandler 是一个处理应用错误的中间件
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 处理请求
		c.Next()

		// 检查是否有错误
		if len(c.Errors) > 0 {
			// 获取最后一个错误
			err := c.Errors.Last().Err

			// 客户端/上游已取消（切页签、组件卸载、请求被后续请求覆盖、代理超时）：
			// 这不是服务端故障——连接通常已断开，写响应也没有接收方。
			// 若按 500 返回，前端会把用户的正常操作当成服务端错误并触发重试
			// （实测列表接口重试 12 次形成风暴，页面长时间刷不出来）。
			// 用 499（nginx 约定：客户端关闭连接）语义化，并降级为 Warn 避免污染错误日志。
			if rerr := c.Request.Context().Err(); rerr != nil {
				logger.Warnf(c.Request.Context(),
					"request canceled by client, suppressing error response: %v %s %s",
					rerr, c.Request.Method, c.Request.URL.Path)
				c.AbortWithStatus(499)
				return
			}

			// 检查是否为应用错误
			if appErr, ok := errors.IsAppError(err); ok {
				// 返回应用错误
				c.JSON(appErr.HTTPCode, gin.H{
					"success": false,
					"error": gin.H{
						"code":    appErr.Code,
						"message": appErr.Message,
						"details": appErr.Details,
					},
				})
				return
			}

			// 处理其他类型的错误
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error": gin.H{
					"code":    errors.ErrInternalServer,
					"message": "Internal server error",
				},
			})
		}
	}
}
