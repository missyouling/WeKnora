// 第 60 轮回归：胶囊状态字段清理、生成账单前置校验、账单只允许手动生成。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
)

// hrRound60Router 建一个带 tenant 上下文的 gin 路由（复用第 59 轮的建库函数）。
// 必须挂 ErrorHandler：hrFail 只 c.Error(...)，HTTP 状态码由错误中间件转换。
func hrRound60Router(t *testing.T, db *HRHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(
			c.Request.Context(), types.TenantIDContextKey, uint64(1)))
		c.Next()
	})
	db.registerHRRoutes(r.Group(""))
	return r
}

func hrRound60Do(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 第 60 轮：启动清理只留「本胶囊白名单」内的加锁状态字段。
func TestRound60RepairCapsuleStatusFields(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "s1", TenantID: 1, Scope: hrScopeSocial, Name: "社保管理", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "个人编号", Group: hrSocReceiptGroup, GroupLabel: hrSocReceiptGroup, Enabled: true},
			{Name: "养老参保状态", Group: hrSocReceiptGroup, GroupLabel: hrSocReceiptGroup, Enabled: true},
			{Name: "账单状态", Group: hrSocReceiptGroup, GroupLabel: hrSocReceiptGroup, Enabled: true},
			{Name: "账单月份", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "养老参保状态", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "生育参保状态", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "账单状态", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "账单月份", Group: hrSocAbnormalGroup, GroupLabel: hrSocAbnormalGroup, Enabled: true},
			{Name: "工伤参保状态", Group: hrSocAbnormalGroup, GroupLabel: hrSocAbnormalGroup, Enabled: true},
			{Name: "账单状态", Group: hrSocAbnormalGroup, GroupLabel: hrSocAbnormalGroup, Enabled: true},
			{Name: "账单周期", Group: hrSocBillGroup, GroupLabel: hrSocBillGroup, Enabled: true},
			{Name: "账单状态", Group: hrSocBillGroup, GroupLabel: hrSocBillGroup, Enabled: true},
			{Name: "账单状态", Group: hrSocBillGroup, GroupLabel: hrSocBillGroup, Enabled: true},
			{Name: "养老参保状态", Group: hrSocBillGroup, GroupLabel: hrSocBillGroup, Enabled: true},
		},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "c1", TenantID: 1, Scope: hrScopeContract, Name: "劳动合同", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "合同编号", Group: hrContractGroupLabor, GroupLabel: hrContractGroupLabor, Enabled: true},
			{Name: "合同状态", Group: hrContractGroupLabor, GroupLabel: hrContractGroupLabor, Enabled: true},
			{Name: "合同状态", Group: hrContractGroupLabor, GroupLabel: hrContractGroupLabor, Enabled: true},
			{Name: "合同状态", Group: hrContractGroupService, GroupLabel: hrContractGroupService, Enabled: true},
			{Name: "账单状态", Group: hrContractGroupService, GroupLabel: hrContractGroupService, Enabled: true},
		},
	}).Error)

	h := &HRHandler{db: db}
	require.NoError(t, h.hrRepair60Once(ctx))

	var social types.FleetCategory
	require.NoError(t, db.Where("id = ?", "s1").First(&social).Error)
	byGroup := map[string][]string{}
	for _, s := range social.Subs {
		byGroup[s.Group] = append(byGroup[s.Group], s.Name)
	}
	require.Equal(t, []string{"个人编号", "养老参保状态"}, byGroup[hrSocReceiptGroup],
		"回盘表只留 5 个五险参保状态，账单状态必须清掉")
	require.Equal(t, []string{"账单月份"}, byGroup[hrSocNormalGroup], "正常记录 0 个加锁字段")
	require.Equal(t, []string{"账单月份"}, byGroup[hrSocAbnormalGroup], "补退记录 0 个加锁字段")
	require.Equal(t, []string{"账单周期", "账单状态"}, byGroup[hrSocBillGroup],
		"月度账单只留 1 个账单状态，五险参保状态清掉")

	var contract types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c1").First(&contract).Error)
	statusCount := map[string]int{}
	for _, s := range contract.Subs {
		if s.Name == "合同状态" {
			statusCount[s.Group]++
			require.True(t, s.Builtin, "保留下来的合同状态必须补 builtin")
		}
		require.NotEqual(t, "账单状态", s.Name, "劳动合同不得有账单状态")
	}
	require.Equal(t, 1, statusCount[hrContractGroupLabor])
	require.Equal(t, 1, statusCount[hrContractGroupService])

	// 幂等
	before := append([]types.FleetCategorySub{}, contract.Subs...)
	require.NoError(t, h.hrRepair60Once(ctx))
	require.NoError(t, db.Where("id = ?", "c1").First(&contract).Error)
	require.Equal(t, before, contract.Subs)
}

// 第 60 轮：precheck 接口返回正常 / 补退条数与账单是否已存在。
func TestRound60PrecheckSIBill(t *testing.T) {
	db := hrRound59DB(t)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "r1", TenantID: 1, BillMonth: "2026-09", RecordType: "normal",
		Name: "张三", IDNumber: "510232198001010011", Scheme: "养老保险", TotalAmount: 100,
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "r2", TenantID: 1, BillMonth: "2026-09", RecordType: "abnormal",
		Name: "张三", IDNumber: "510232198001010011", Scheme: "养老保险", TotalAmount: 10,
	}).Error)
	require.NoError(t, db.Create(&HRSIBill{
		ID: "b1", TenantID: 1, MonthLabel: "2026-09", BillStatus: "已申报",
	}).Error)

	h := &HRHandler{db: db}
	r := hrRound60Router(t, h)

	w := hrRound60Do(t, r, http.MethodGet, "/hr/social/bills/precheck?month=2026-09", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Month       string `json:"month"`
			NormalCount int64  `json:"normal_record_count"`
			AdjustCount int64  `json:"adjust_record_count"`
			BillExists  bool   `json:"bill_exists"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "2026-09", resp.Data.Month)
	require.Equal(t, int64(1), resp.Data.NormalCount)
	require.Equal(t, int64(1), resp.Data.AdjustCount)
	require.True(t, resp.Data.BillExists)

	w = hrRound60Do(t, r, http.MethodGet, "/hr/social/bills/precheck?month=2026-10", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, int64(0), resp.Data.NormalCount)
	require.False(t, resp.Data.BillExists)

	// 非法月份
	w = hrRound60Do(t, r, http.MethodGet, "/hr/social/bills/precheck?month=2026", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// 第 60 轮：没有正常记录的月份不得生成空账单（前后端口径一致）。
func TestRound60CreateSIBillRequiresNormalRecords(t *testing.T) {
	db := hrRound59DB(t)
	h := &HRHandler{db: db}
	r := hrRound60Router(t, h)

	// 只有补退记录：仍然拒绝
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "r1", TenantID: 1, BillMonth: "2026-08", RecordType: "abnormal",
		Name: "李四", IDNumber: "510232198002020022", Scheme: "养老保险", TotalAmount: 10,
	}).Error)
	w := hrRound60Do(t, r, http.MethodPost, "/hr/social/bills", map[string]any{"month_label": "2026-08"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "该月份暂无正常社保记录，请先上传社保系统导出的记录数据")
	var n int64
	require.NoError(t, db.Model(&HRSIBill{}).Where("tenant_id = ? AND month_label = ?", 1, "2026-08").Count(&n).Error)
	require.Equal(t, int64(0), n, "被拦截时绝不能留下空账单")

	// 补齐正常记录后可生成
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "r2", TenantID: 1, BillMonth: "2026-08", RecordType: "normal",
		Name: "李四", IDNumber: "510232198002020022", Scheme: "养老保险",
		Base: 5000, UnitAmount: 800, PersonalAmount: 400, TotalAmount: 1200,
	}).Error)
	w = hrRound60Do(t, r, http.MethodPost, "/hr/social/bills", map[string]any{"month_label": "2026-08"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var bill HRSIBill
	require.NoError(t, db.Where("tenant_id = ? AND month_label = ?", 1, "2026-08").First(&bill).Error)
	require.Equal(t, 1, bill.Headcount)
	require.InDelta(t, 800, bill.UnitTotal, 0.001)
	require.InDelta(t, 400, bill.PersonalTotal, 0.001)

	// 同月再生成且未 overwrite → 400（前端必须先弹覆盖确认）
	w = hrRound60Do(t, r, http.MethodPost, "/hr/social/bills", map[string]any{"month_label": "2026-08"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "已存在社保月度账单")

	// overwrite=true 才覆盖
	w = hrRound60Do(t, r, http.MethodPost, "/hr/social/bills", map[string]any{"month_label": "2026-08", "overwrite": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// 第 60 轮铁律：账单只能手动生成 —— 导入 / 启动链路不得调用生成逻辑。
func TestRound60NoAutoBillGeneration(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(name)
		require.NoError(t, err)
		return string(b)
	}
	importSrc := read("hr_social_round56.go")
	body := func(fn string) string {
		start := regexp.MustCompile(`func \(h \*HRHandler\) ` + fn + `\(`).FindStringIndex(importSrc)
		if start == nil {
			return ""
		}
		rest := importSrc[start[0]:]
		end := regexp.MustCompile(`\nfunc \(h \*HRHandler\) `).FindStringIndex(rest[1:])
		if end == nil {
			return rest
		}
		return rest[:end[0]+1]
	}
	for _, fn := range []string{"ImportSIRecords", "ImportSIReceipts"} {
		src := body(fn)
		require.NotEmpty(t, src, fn+" 必须存在")
		require.NotContains(t, src, "CreateSIBill", fn+" 不得自动生成月度账单")
		require.NotContains(t, src, "hrSIBillTotalsOf", fn+" 不得自动汇总账单")
	}
	// 启动种子只在首次初始化时落 2026-11 样例账单（seeded 门禁在 hr_bootstrap.go）。
	boot := read("hr_bootstrap.go")
	require.Contains(t, boot, "hrInitialSeeded(ctx)")
	require.Contains(t, boot, "step(h.hrSeedSocialRound56(ctx)")
}
