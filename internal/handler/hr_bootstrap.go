// 人事三模块启动装配：init 复用 fleet 分类 scope、9 表 AutoMigrate、幂等种子、
// 单位默认配置，以及挂在 /api/v1/fleet/hr 下的全部路由。
//
// 唯一挂载点是 routes_knowledge.go 的 RegisterFleetRoutes 里一行
// handler.BindHRRoutes(fl)（[upstream-touch]）；本文件不改任何既有文件。
package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// HR 三个分类 scope；init 里扩展同包 fleetCategoryScopes，零修改既有文件即可
// 复用 /api/v1/fleet/categories 的「字段定义 / 列表列 / 筛选器 / 抽屉字段 / 提取规则」五同步。
const (
	hrScopeRoster   = "hr-roster"
	hrScopeContract = "hr-contract"
	hrScopeSocial   = "hr-social"
	hrScopeFund     = "hr-fund"
)

// 单位公积金默认配置（源项目 api/provident.go:18-20）。
const (
	hrDefaultPFUnitName    = "重庆星达铜业有限公司"
	hrDefaultPFUnitAccount = "201005128130"
	hrDefaultPFRatio       = 6.0
)

// init 扩展分类 scope 白名单（HR 走租户隔离，不动 isBusinessCategoryScope）。
func init() {
	fleetCategoryScopes[hrScopeRoster] = true
	fleetCategoryScopes[hrScopeContract] = true
	fleetCategoryScopes[hrScopeSocial] = true
	fleetCategoryScopes[hrScopeFund] = true
}

// HRHandler 人事三模块处理器。
type HRHandler struct {
	db *gorm.DB
}

// NewHRHandler 建 9 张表（best-effort，失败只 Warn 不 panic）+ 幂等分类种子 + 单位默认配置。
func NewHRHandler(db *gorm.DB) *HRHandler {
	h := &HRHandler{db: db}
	if err := db.AutoMigrate(
		&HREmployee{}, &HRSIPeriod{}, &HRSIRawRecord{}, &HRSICharge{}, &HRSIChange{},
		&HRPFSettings{}, &HRPFRecord{}, &HRPFBill{}, &HRPFBillItem{},
	); err != nil {
		logger.Warnf(context.Background(), "AutoMigrate HR tables failed: %v", err)
	}
	// 第 47 轮：组织机构三表
	hrOrgAutoMigrate(db)
	ctx := context.Background()
	h.seedHRCategories(ctx)
	h.seedHRPFSettings(ctx)
	return h
}

// hrTenantID 统一取租户 ID（int64，与 types.FleetCategory.TenantID 同口径）。
func hrTenantID(c *gin.Context) int64 {
	return int64(types.MustTenantIDFromContext(c.Request.Context()))
}

// hrOK / hrFail / hrConflict 统一响应与错误口径。
func hrOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func hrFail(c *gin.Context, msg string) {
	c.Error(errors.NewBadRequestError(msg))
}

func hrConflict(c *gin.Context, conflicts any) {
	c.JSON(http.StatusConflict, gin.H{"success": false, "conflicts": conflicts})
}

// ---------------- 分类种子 ----------------

type hrCategorySeed struct {
	Scope      string
	Name       string
	BuiltinKey string
	SortOrder  int
	Subs       []types.FleetCategorySub
}

// hrSub 统一：Width=0（自适应）、Enabled=true、Builtin=true（状态字段锁定）。
func hrSub(s types.FleetCategorySub) types.FleetCategorySub {
	s.Width = 0
	s.Enabled = true
	s.Builtin = true
	return s
}

// hrRosterSeedSubs 是第 46 轮花名册字段种子：76 个标准模板字段（按「说明」Sheet
// 11 大分组顺序）+ 派生字段 + 2 个离职扩展字段。
//
// 故意**不写 Group**：FleetCertPanel 会按 group 分页签过滤字段行，HR 走平铺更安全
// （第 45 轮市电账单踩过「状态字段无 group 被过滤掉」的坑）。分组信息保留在
// hrRosterFieldGroups 里，用于导入模板的「说明」Sheet。
func hrRosterSeedSubs() []types.FleetCategorySub {
	out := make([]types.FleetCategorySub, 0, 82)
	seen := make(map[string]bool, 82)
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		s := types.FleetCategorySub{
			Name:      name,
			Enabled:   true,
			IsDefault: false, // 列显隐由四态视图决定，默认不把 78 列全塞进列表
			Width:     0,     // 0 = 自适应，禁止回跳 160
		}
		switch {
		case hrIsRosterDateField(name):
			s.DataType = "date"
		case strings.Contains(name, "金额") || strings.Contains(name, "基数") || strings.Contains(name, "卡号"):
			s.DataType = "number"
		default:
			s.DataType = "text"
		}
		// 只有系统状态类字段锁死（禁止删除/改名），业务字段允许用户在设置里微调。
		s.Builtin = name == "员工状态" || name == "在职状态" || name == "缴存状态" || name == "账期状态"
		out = append(out, s)
	}
	for _, g := range hrRosterFieldGroups {
		for _, name := range g.Fields {
			add(name)
		}
	}
	for _, name := range hrRosterDerivedFields {
		add(name)
	}
	for _, name := range hrRosterExtraResignFields {
		add(name)
	}
	return out
}

// hrContractSeedSubs 劳动合同视图的默认字段（第 47 轮）。
// 与花名册合同信息同源：数据都来自 hr_employees 的 custom_data，只是换个投影。
func hrContractSeedSubs() []types.FleetCategorySub {
	names := []string{
		"工号", "姓名", "部门", "职务", "岗位", "证件号码", "入职日期",
		"合同签约公司", "现合同编号", "现合同类型", "现合同开始日期",
		"现合同结束日期", "现合同期限", "现合同签订日期", "合同状态",
	}
	out := make([]types.FleetCategorySub, 0, len(names))
	for _, name := range names {
		s := types.FleetCategorySub{Name: name, Enabled: true, Width: 0}
		switch {
		case hrIsRosterDateField(name):
			s.DataType = "date"
		default:
			s.DataType = "text"
		}
		// 合同状态是系统内置字段：不可删、不可改名与类型，但可拖动排序。
		s.Builtin = name == "合同状态"
		out = append(out, s)
	}
	return out
}

// hrCategorySeeds 四个 scope 各 1 个分类。
func hrCategorySeeds() []hrCategorySeed {
	return []hrCategorySeed{
		{
			Scope: hrScopeRoster, Name: "花名册", BuiltinKey: "hr-roster", SortOrder: 1,
			Subs: hrRosterSeedSubs(),
		},
		{
			Scope: hrScopeContract, Name: "劳动合同", BuiltinKey: "hr-contract", SortOrder: 2,
			Subs: hrContractSeedSubs(),
		},
		{
			Scope: hrScopeSocial, Name: "社保管理", BuiltinKey: "hr-social", SortOrder: 1,
			Subs: []types.FleetCategorySub{
				hrSub(txt("账期")), hrSub(txt("姓名")), hrSub(txt("身份证号码")), hrSub(txt("部门")),
				hrSub(num("缴费基数")), hrSub(num("养老")), hrSub(num("医疗")), hrSub(num("大病")),
				hrSub(num("失业")), hrSub(num("工伤")), hrSub(num("小计")), hrSub(txt("参保状态")),
			},
		},
		{
			Scope: hrScopeFund, Name: "公积金管理", BuiltinKey: "hr-fund", SortOrder: 1,
			Subs: []types.FleetCategorySub{
				hrSub(txt("个人账号")), hrSub(txt("姓名")), hrSub(txt("身份证号码")), hrSub(num("缴存基数")),
				hrSub(num("缴存比例")), hrSub(num("个人金额")), hrSub(num("单位金额")), hrSub(num("合计")),
				hrSub(txt("缴存状态")), hrSub(area("备注")),
			},
		},
	}
}

// hrSeedTenantIDs = {0} + 已有分类的租户（与 fleet_record_category_seed.go 同口径）。
func hrSeedTenantIDs(ctx context.Context, db *gorm.DB) []int64 {
	tenantIDs := []int64{0}
	var existing []int64
	if err := db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("deleted_at IS NULL AND tenant_id > 0").
		Distinct().Pluck("tenant_id", &existing).Error; err == nil {
		tenantIDs = append(tenantIDs, existing...)
	}
	return tenantIDs
}

// seedHRCategories 幂等写 3 个分类：按 (tenant_id, scope, name) 查，存在则只
// mergeCategorySubs 补缺失字段并 Save(&row)，绝不覆盖用户顺序/宽度/启停。
func (h *HRHandler) seedHRCategories(ctx context.Context) {
	for _, tenantID := range hrSeedTenantIDs(ctx, h.db) {
		for _, seed := range hrCategorySeeds() {
			var rows []types.FleetCategory
			err := h.db.WithContext(ctx).
				Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL",
					tenantID, seed.Scope, seed.Name).
				Find(&rows).Error
			if err != nil {
				logger.Warnf(ctx, "seed HR category %s/%s: query failed: %v", seed.Scope, seed.Name, err)
				continue
			}
			if len(rows) == 0 {
				cat := types.FleetCategory{
					ID:         uuid.New().String(),
					TenantID:   tenantID,
					Scope:      seed.Scope,
					Name:       seed.Name,
					BuiltinKey: seed.BuiltinKey,
					Subs:       seed.Subs,
					SortOrder:  seed.SortOrder,
					Enabled:    true,
				}
				if cerr := h.db.WithContext(ctx).Create(&cat).Error; cerr != nil {
					logger.Warnf(ctx, "seed HR category %s/%s: create failed: %v", seed.Scope, seed.Name, cerr)
				}
				continue
			}
			for i := range rows {
				merged, added := mergeCategorySubs(rows[i].Subs, seed.Subs)
				if added == 0 {
					continue
				}
				// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
				rows[i].Subs = merged
				if uerr := h.db.WithContext(ctx).Save(&rows[i]).Error; uerr != nil {
					logger.Warnf(ctx, "seed HR category %s/%s: backfill failed: %v", seed.Scope, seed.Name, uerr)
				}
			}
		}
	}
}

// seedHRPFSettings 幂等写单位默认配置（已存在则不覆盖用户修改）。
func (h *HRHandler) seedHRPFSettings(ctx context.Context) {
	for _, tenantID := range hrSeedTenantIDs(ctx, h.db) {
		var existing HRPFSettings
		err := h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&existing).Error
		if err == nil {
			continue
		}
		row := HRPFSettings{
			ID:           uuid.New().String(),
			TenantID:     tenantID,
			UnitName:     hrDefaultPFUnitName,
			UnitAccount:  hrDefaultPFUnitAccount,
			DefaultRatio: hrDefaultPFRatio,
		}
		if cerr := h.db.WithContext(ctx).Create(&row).Error; cerr != nil {
			logger.Warnf(ctx, "seed HR PF settings for tenant %d failed: %v", tenantID, cerr)
		}
	}
}

// ---------------- 路由绑定 ----------------

// BindHRRoutes 由 RegisterFleetRoutes 调用；路由组前缀 /hr
// （fl 组前缀 /api/v1/fleet → 完整 /api/v1/fleet/hr）。
//
// 这里**不做单例缓存**：NewRouter 在单测里会用一个 sqlite 内存库调用一次、
// 生产又用真实 PG 调一次，sync.Once 会把第一个测试的 db 永久绑死。
// AutoMigrate 与分类/单位种子本身幂等，重复执行无害。
// hrRouteRegistrar 是路由注册器的最小抽象。
//
// router 包的 `fl` 实际是 `*apiKeyRouteGroup`（router 包私有类型），不是
// `*gin.RouterGroup`；但它与 gin.RouterGroup 的 GET/POST/PUT/DELETE 签名完全一致，
// 故用接口收口：HR 路由自动继承 fl 组的 apiKeyRetrieve(full-access) 策略，
// 又不必引用 router 包的私有类型。测试可直接传 `*gin.RouterGroup`。
type hrRouteRegistrar interface {
	GET(string, ...gin.HandlerFunc) gin.IRoutes
	POST(string, ...gin.HandlerFunc) gin.IRoutes
	PUT(string, ...gin.HandlerFunc) gin.IRoutes
	DELETE(string, ...gin.HandlerFunc) gin.IRoutes
}

func (h *FleetHandler) BindHRRoutes(r hrRouteRegistrar) {
	NewHRHandler(h.db).registerHRRoutes(r)
}

// registerHRRoutes 注册全部 HR 路由；测试可直接用 NewHRHandler 实例调用。
func (h *HRHandler) registerHRRoutes(r hrRouteRegistrar) {
	// 花名册
	r.GET("/hr/roster/employees", h.ListEmployees)
	r.POST("/hr/roster/employees", h.CreateEmployee)
	r.PUT("/hr/roster/employees/:id", h.UpdateEmployee)
	r.DELETE("/hr/roster/employees/:id", h.DeleteEmployee)
	r.POST("/hr/roster/employees/batch-delete", h.BatchDeleteEmployees)
	r.POST("/hr/roster/employees/resign", h.ResignEmployees)
	r.POST("/hr/roster/employees/restore", h.RestoreEmployees)
	r.POST("/hr/roster/import", h.ImportRoster)
	r.GET("/hr/roster/export", h.ExportRoster)
	r.GET("/hr/roster/stats", h.RosterStats)
	// 第 46 轮：导入模板下载、离职证明上传与在线预览
	r.GET("/hr/roster/template", h.DownloadRosterTemplate)
	r.POST("/hr/roster/resign-proof", h.UploadResignProof)
	r.GET("/hr/roster/resign-proof/:id", h.GetResignProof)

	// 社保
	r.GET("/hr/social/periods", h.ListPeriods)
	r.POST("/hr/social/periods", h.CreatePeriod)
	r.DELETE("/hr/social/periods/:id", h.DeletePeriod)
	r.POST("/hr/social/periods/:id/import", h.ImportSocialFile)
	// 第 47 轮：多文件自适应导入 + 原始/汇总双视图
	r.POST("/hr/social/import", h.ImportSocialBatch)
	r.GET("/hr/social/raw", h.ListSIRawRecords)
	r.POST("/hr/social/summary", h.GenerateSISummary)
	r.GET("/hr/social/periods/:id/detail", h.GetSIPeriodDetail)
	r.POST("/hr/social/periods/:id/process", h.ProcessPeriod)
	r.POST("/hr/social/periods/:id/process-adjustments", h.ProcessAdjustments)
	r.GET("/hr/social/periods/:id/charges", h.ListCharges)
	r.GET("/hr/social/periods/:id/export", h.ExportCharges)
	r.GET("/hr/social/periods/:id/changes", h.ListChanges)
	r.POST("/hr/social/periods/:id/changes", h.CreateChange)
	r.PUT("/hr/social/periods/:id/changes/:changeId", h.UpdateChange)
	r.DELETE("/hr/social/periods/:id/changes/:changeId", h.DeleteChange)

	// 公积金
	r.GET("/hr/fund/settings", h.GetPFSettings)
	r.PUT("/hr/fund/settings", h.UpdatePFSettings)
	r.GET("/hr/fund/records", h.ListPFRecords)
	r.POST("/hr/fund/records", h.CreatePFRecord)
	r.PUT("/hr/fund/records/:id", h.UpdatePFRecord)
	r.DELETE("/hr/fund/records/:id", h.DeletePFRecord)
	r.POST("/hr/fund/records/:id/seal", h.SealPFRecord)
	r.POST("/hr/fund/records/:id/unseal", h.UnsealPFRecord)
	r.POST("/hr/fund/import", h.ImportPFRecords)
	r.GET("/hr/fund/export", h.ExportPFRecords)
	r.GET("/hr/fund/bills", h.ListPFBills)
	r.POST("/hr/fund/bills", h.CreatePFBill)
	r.GET("/hr/fund/bills/:id", h.GetPFBill)
	r.DELETE("/hr/fund/bills/:id", h.DeletePFBill)

	// 第 48 轮：新增员工时的工号自动编码建议
	r.GET("/hr/roster/next-employee-id", h.NextEmployeeID)
	// 第 47 轮：劳动合同视图 + 组织机构设置
	r.GET("/hr/contracts", h.ListContracts)
	r.GET("/hr/org", h.GetOrganization)
	r.PUT("/hr/org", h.SaveOrganization)
}
