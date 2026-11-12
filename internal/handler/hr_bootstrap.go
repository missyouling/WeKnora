// 人事三模块启动装配：init 复用 fleet 分类 scope、9 表 AutoMigrate、幂等种子、
// 单位默认配置，以及挂在 /api/v1/fleet/hr 下的全部路由。
//
// 唯一挂载点是 routes_knowledge.go 的 RegisterFleetRoutes 里一行
// handler.BindHRRoutes(fl)（[upstream-touch]）；本文件不改任何既有文件。
package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

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
	hrScopeResign   = "hr-resign"
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
	fleetCategoryScopes[hrScopeResign] = true // 第 52 轮：离职管理独立卡片
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
	// 第 54 轮：公积金字段配置重建为网厅 18 字段（幂等；旧 10 字段 merge 后会残留）
	if err := h.hrMigrateFundSubs(ctx); err != nil {
		logger.Warnf(ctx, "migrate HR fund subs failed: %v", err)
	}
	// 第 54 轮：花名册「个人社保账号 / 个人公积金账号」升级为只读跨模块引用
	if err := h.hrMigrateRosterRefSubs(ctx); err != nil {
		logger.Warnf(ctx, "migrate HR roster ref subs failed: %v", err)
	}
	// 第 50 轮：启动时归并存量旧部门名（质量部 → 技术质量部 等）。
	// 幂等且失败只告警：宁可留脏数据，也不能让服务起不来。
	if err := h.hrNormalizeAllDepartments(ctx); err != nil {
		logger.Warnf(ctx, "normalize HR departments failed: %v", err)
	}
	// 第 53 轮：存量员工司龄批量回填（幂等，只在值变化时写库）。
	// 列表响应层也会实时重算，这里落库是为了导出/详情等离线读取也有值。
	if err := h.hrBackfillSeniority(ctx); err != nil {
		logger.Warnf(ctx, "backfill HR seniority failed: %v", err)
	}
	// 第 54 轮：离职样例 + 公积金网厅 87 条（均幂等，失败只告警不阻断启动）。
	if err := h.hrSeedResignSamples(ctx); err != nil {
		logger.Warnf(ctx, "seed HR resign samples failed: %v", err)
	}
	if err := h.hrSeedNewHireSamples(ctx); err != nil {
		logger.Warnf(ctx, "seed HR new hire samples failed: %v", err)
	}
	if err := h.hrSeedFundRecords(ctx); err != nil {
		logger.Warnf(ctx, "seed HR fund records failed: %v", err)
	}
	return h
}

// hrBackfillSeniority 批量回填存量员工 custom_data["司龄"]（幂等）。
//
// 规则与 hrSeniorityOf 完全一致：从入职日期起算，离职者截至离职日期，
// 格式 X年Y个月；没有入职日期的行跳过（不造假值）。
func (h *HRHandler) hrBackfillSeniority(ctx context.Context) error {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, tid := range tids {
		var rows []HREmployee
		if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&rows).Error; err != nil {
			continue
		}
		for i := range rows {
			want := hrSeniorityOf(&rows[i], now)
			if want == "" {
				continue
			}
			if rows[i].CustomData == nil {
				rows[i].CustomData = map[string]any{}
			}
			if strings.TrimSpace(fmt.Sprint(rows[i].CustomData["司龄"])) == want {
				continue
			}
			rows[i].CustomData["司龄"] = want
			// custom_data 是 serializer:json 的 JSONB，必须走结构体 Save
			// （Updates(map) 在 PG 会报 42804）。
			if err := h.db.WithContext(ctx).Save(&rows[i]).Error; err != nil {
				continue
			}
		}
	}
	return nil
}

// hrNormalizeAllDepartments 对每个租户跑一次部门名归并（启动钩子用）。
func (h *HRHandler) hrNormalizeAllDepartments(ctx context.Context) error {
	var tids []int64
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Distinct().Pluck("tenant_id", &tids).Error; err != nil {
		return err
	}
	for _, tid := range tids {
		h.hrEnsureOrgDefaults(ctx, tid)
		h.hrEnsureOrgUnits(ctx, tid)
		h.hrMigrateOrgCodes(ctx, tid)
		if _, err := h.hrRefreshOrgDepartments(ctx, tid); err != nil {
			return err
		}
	}
	return nil
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

// hrRosterGroupDict 是精简后的 6 大分组 → 字段名清单（第 52 轮）。
//
// 与前端 hrFieldSchema.ts / hrRosterGroups.ts 同源同序。Group/GroupLabel 写进
// subs 仅用于「编辑抽屉按 6 组分块」；花名册的「设置 → 字段设置」已通过
// hideGroupTabs 取消胶囊栏，字段按 subs 顺序合并成单一连续列表。
//
// 第 52 轮删除：家庭情况 / 工作经历情况 / 个税申报信息 三组全部字段；
// 剥离：离职信息 9 字段 → hr-resign，合同签订情况 8 字段 → hr-contract；
// 删除字段：在职状态 / 人员状态。
var hrRosterGroupDict = hrRosterGroupDict52()

// hrRosterDefaultHeaderFields 默认表头开关（字段契约里 defaultHeader=true 的字段）。
var hrRosterDefaultHeaderFields = func() map[string]bool {
	m := make(map[string]bool, 24)
	for _, d := range hrRosterFieldDefs() {
		if d.DefaultHeader {
			m[d.Name] = true
		}
	}
	return m
}()

// hrRosterRetiredFields 是被合并 / 下线的历史字段（第 49 轮）：
// 生日 → 出生日期；工龄 / 司龄开始日期 / 司龄(天数) → 司龄。
// hrRosterSystemStatusFields 是系统注入的状态字段：只归组，不参与
// 「默认表头」重写（该开关由前端 statusFields.ts 维护）。
var hrRosterSystemStatusFields = map[string]bool{
	"在职状态": true, "证件状态": true, "证照状态": true, "提取状态": true,
}

var hrRosterRetiredFields = map[string]bool{
	// 合并：生日 → 出生日期；工龄/司龄开始日期/司龄(天数) → 司龄
	"生日": true, "工龄": true, "司龄开始日期": true, "司龄(天数)": true,
	// 历史重复列（第 46 轮导入别名遗留）：统一到标准表头
	"身份证号码": true, "出生月份": true, "联系电话": true, "在职状态": true,
}

// hrRosterGroupOf 字段 → 所属分组 key（未收录时归「基本信息」以外的兜底组）。
func hrRosterGroupOf(name string) (string, bool) {
	for _, g := range hrRosterGroupDict {
		for _, f := range g.Fields {
			if f == name {
				return g.Key, true
			}
		}
	}
	return "", false
}

// hrRosterSeedSubs 第 52 轮：精简后的 48 个在职字段，来自字段契约单一数据源。
// 日期字段自动是 date 类型，枚举字段仍是文本值（前端按 options 渲染下拉）。
func hrRosterSeedSubs() []types.FleetCategorySub {
	return hrRosterSeedSubs52()
}

// hrContractSeedSubs 第 52 轮：劳动合同 8 核心字段 + 6 乙方引用 + 3 甲方引用。
func hrContractSeedSubs() []types.FleetCategorySub {
	return hrContractSeedSubs52()
}

// hrResignSubs 第 52 轮：离职管理 9 核心字段 + 5 花名册引用字段。
func hrResignSubs() []types.FleetCategorySub {
	return hrResignSeedSubs()
}

// hrBackfillRosterGroups 把存量的花名册 subs 重写成 11 分组口径（幂等）。
//
// seedHRCategories 只在「分类缺失」时新增，已存在的分类不会补 group；
// 第 48 轮之前建的租户因此看不到胶囊分组。这里按字段名逐个回填：
//
//	· Group/GroupLabel = 11 组之一
//	· IsDefault = 基本信息组 + 核心锚点（工号/姓名/部门/岗位）
//	· Enabled 保持 true（详情抽屉按分组展示）
func hrBackfillRosterGroups(cats []*types.FleetCategory) bool {
	changed := false
	for _, c := range cats {
		if c == nil {
			continue
		}
		var legacy map[string]bool
		var seed []types.FleetCategorySub
		switch c.BuiltinKey {
		case "hr-roster":
			legacy, seed = hrLegacyRosterFields, hrRosterSeedSubs52()
		case "hr-resign":
			seed = hrResignSeedSubs()
		case "hr-contract":
			legacy, seed = hrLegacyContractFields, hrContractSeedSubs52()
		default:
			continue
		}
		byName := make(map[string]types.FleetCategorySub, len(seed))
		rank := make(map[string]int, len(seed))
		for i, s := range seed {
			byName[s.Name] = s
			rank[s.Name] = i
		}
		subs := c.Subs
		kept := make([]types.FleetCategorySub, 0, len(subs))
		for i := range subs {
			name := strings.TrimSpace(subs[i].Name)
			if name == "" {
				changed = true
				continue
			}
			// 第 52 轮：剔除已删除分组 / 已剥离模块 / 已改名的旧内置字段。
			// 带 source 的用户引用字段与未识别的自定义字段一律保留。
			if legacy != nil && legacy[name] && subs[i].Source == nil {
				changed = true
				continue
			}
			if want, ok := byName[name]; ok {
				// 按字段契约回填 group / 类型 / 默认表头 / 内置锁 / 引用来源。
				// 用户自改的显示名（label）、启用状态与顺序不覆盖。
				if subs[i].Group != want.Group || subs[i].GroupLabel != want.GroupLabel {
					subs[i].Group, subs[i].GroupLabel = want.Group, want.GroupLabel
					changed = true
				}
				if subs[i].DataType != want.DataType {
					subs[i].DataType = want.DataType
					changed = true
				}
				if subs[i].IsDefault != want.IsDefault {
					subs[i].IsDefault = want.IsDefault
					changed = true
				}
				if want.Builtin && !subs[i].Builtin {
					subs[i].Builtin = true
					changed = true
				}
				if want.Source != nil && subs[i].Source == nil {
					src := *want.Source
					subs[i].Source = &src
					changed = true
				}
			}
			if !subs[i].Enabled {
				subs[i].Enabled = true
				changed = true
			}
			if subs[i].Width != 0 {
				subs[i].Width = 0
				changed = true
			}
			kept = append(kept, subs[i])
		}
		// 按字段契约顺序重排（花名册即 6 大分组顺序）；用户新增字段追加末尾。
		sortedSubs := make([]types.FleetCategorySub, len(kept))
		copy(sortedSubs, kept)
		sort.SliceStable(sortedSubs, func(a, b int) bool {
			ra, oka := rank[sortedSubs[a].Name]
			rb, okb := rank[sortedSubs[b].Name]
			if !oka {
				ra = len(rank)
			}
			if !okb {
				rb = len(rank)
			}
			return ra < rb
		})
		for i := range sortedSubs {
			if sortedSubs[i].Name != kept[i].Name {
				changed = true
				break
			}
		}
		c.Subs = sortedSubs
	}
	return changed
}

// hrCategorySeeds 五个 scope 各 1 个分类（第 52 轮新增离职管理）。
func hrCategorySeeds() []hrCategorySeed {
	return []hrCategorySeed{
		{
			Scope: hrScopeRoster, Name: "花名册", BuiltinKey: "hr-roster", SortOrder: 1,
			Subs: hrRosterSeedSubs(),
		},
		{
			Scope: hrScopeResign, Name: "离职管理", BuiltinKey: "hr-resign", SortOrder: 2,
			Subs: hrResignSubs(),
		},
		{
			Scope: hrScopeContract, Name: "劳动合同", BuiltinKey: "hr-contract", SortOrder: 3,
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
			Subs: hrFundSeedSubs(),
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
				rows[i].Subs = merged
				// 第 49 轮：按字段名回填 11 分组 / 默认表头 / 列宽（幂等）。
				patched := hrBackfillRosterGroups([]*types.FleetCategory{&rows[i]})
				if added == 0 && !patched {
					continue
				}
				// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
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
	// 第 52 轮：/hr/roster/template?module=roster|resign|contract 返回各自专属模板
	r.GET("/hr/roster/template", h.DownloadRosterTemplate)
	r.POST("/hr/roster/resign-proof", h.UploadResignProof)
	r.GET("/hr/roster/resign-proof/:id", h.GetResignProof)
	// 第 52 轮：离职管理 / 劳动合同独立导入解析器
	r.POST("/hr/resign/import", h.ImportResignModule)
	r.POST("/hr/contract/import", h.ImportContractModule)

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
	// 第 49 轮：多单位组织机构（单位可无下属部门）
	r.GET("/hr/org/units", h.GetOrgUnits)
	r.PUT("/hr/org/units", h.SaveOrgUnits)
}
