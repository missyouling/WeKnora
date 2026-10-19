package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeUserService 只覆写 ValidateToken，其余方法由嵌入的 nil 接口兜底。
type fakeUserService struct {
	interfaces.UserService
	err error
}

func (f *fakeUserService) ValidateToken(ctx context.Context, token string) (*types.User, uint64, error) {
	return nil, 0, f.err
}

// TestAuthBackendUnavailableReturns503Not401 锁定鉴权故障的错误分类：
//
//   - 鉴权存储不可用（连接池打满 / DB 断连）→ 503，前端会重试并**保留登录态**；
//   - token 真失效 → 401，前端才清凭据跳登录。
//
// 实测事故：Supabase session 模式连接池上限 15 被打满，鉴权 SQL 报
// EMAXCONNSESSION，中间件一律回 401 → 前端清 token 跳登录，用户在一次
// 50 秒解析轮询里被踢下线数次。
func TestAuthBackendUnavailableReturns503Not401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "鉴权存储不可用 → 503",
			err:      fmt.Errorf("%w: look up token: connection refused", types.ErrTokenStoreUnavailable),
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name:     "token 真失效 → 401",
			err:      fmt.Errorf("token is revoked"),
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "无效签名 → 401",
			err:      fmt.Errorf("invalid token"),
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(Auth(nil, &fakeUserService{err: tc.err}, nil, nil, nil))
			r.GET("/api/v1/knowledge-bases/kb1/invoices", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"success": true})
			})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb1/invoices", nil)
			req.Header.Set("Authorization", "Bearer some-token")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.wantCode, w.Code, "鉴权错误分类错误：%v", tc.err)
			if tc.wantCode == http.StatusServiceUnavailable {
				// 必须带可识别标记，便于前端/网关区分「临时故障」与「登录失效」
				assert.Contains(t, w.Body.String(), "auth_backend_unavailable")
			}
		})
	}
}

// TestAuthBackendUnavailableWithoutBearerStays401：没有携带 Bearer 时
// 不应因为存储不可用而回 503（根本没有鉴权动作）。
func TestAuthBackendUnavailableWithoutBearerStays401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Auth(nil, &fakeUserService{err: types.ErrTokenStoreUnavailable}, nil, nil, nil))
	r.GET("/api/v1/knowledge-bases/kb1/invoices", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb1/invoices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
