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
	// 第 61 轮：宿舍管理（五胶囊）
	fleetCategoryScopes[hrScopeDorm] = true
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
		// 第 56 轮：社保四视图（回盘表 / 正常记录 / 异常记录 / 月度账单）
		&HRSIReceipt{}, &HRSIRecord{}, &HRSIBill{}, &HRSIBillItem{},
		// 第 61 轮：离职证明二进制入库（生产容器重建后仍可预览）
		&HRResignProofFile{},
	); err != nil {
		logger.Warnf(context.Background(), "AutoMigrate HR tables failed: %v", err)
	}
	// 第 61 轮：宿舍管理四表（记录 / 账单 / 明细 / 表计读数）
	hrDormAutoMigrate(db)
	// 第 47 轮：组织机构三表
	hrOrgAutoMigrate(db)
	ctx := context.Background()
	// 第 59 轮：标记存放于 system_settings（迁移 000053）。单测的内存库没有跑迁移，
	// 只在缺表时补建，绝不对既有表做结构变更。
	if !db.Migrator().HasTable(&types.SystemSetting{}) {
		if err := db.AutoMigrate(&types.SystemSetting{}); err != nil {
			logger.Warnf(ctx, "AutoMigrate system_settings failed: %v", err)
		}
	}
	// 第 59 轮：首次初始化完成标记。缺失 = 首次启动，跑完整迁移 + 业务种子；
	// 存在 = 用户已经在用，只做幂等修复，绝不把删掉的字段 / 记录加回来。
	seeded := h.hrInitialSeeded(ctx)
	// 第 59 轮：旧胶囊名 → 新胶囊名（本身就是修复，不受首次初始化门禁限制）。
	// 放在全部迁移之前：存量分类改名后，迁移函数里的「离职人员」判定才能命中旧配置。
	if err := h.hrRenameGroups59(ctx); err != nil {
		logger.Warnf(ctx, "rename HR field groups failed: %v", err)
	}
	h.seedHRCategories(ctx, !seeded)
	h.seedHRPFSettings(ctx)
	if seeded {
		// 第 59 轮：安全修复 —— 清重复「合同状态」与串入正常/补退胶囊的参保状态字段。
		if err := h.hrRepair58Once(ctx); err != nil {
			logger.Warnf(ctx, "repair HR field config failed: %v", err)
		}
		// 第 68 轮：已在用的租户同样要跑的三项幂等修复（新增字段 / 补快照 / 唯一索引）。
		// 它们只做「补缺」，绝不清空或回滚用户删除过的字段与数据。
		h.hrRepair68(ctx)
	} else {
		initOK := true
		step := func(err error, msg string) {
			if err != nil {
				logger.Warnf(ctx, msg, err)
				initOK = false
			}
		}
		// 第 54 轮：公积金字段配置重建为网厅 18 字段（幂等；旧 10 字段 merge 后会残留）
		step(h.hrMigrateFundSubs(ctx), "migrate HR fund subs failed: %v")
		// 第 56 轮：社保字段配置重建为四胶囊 76 字段（旧 12 个静态 subs 会被清掉）
		step(h.hrMigrateSocialSubs(ctx), "migrate HR social subs failed: %v")
		// 第 57 轮：花名册字段配置扩展为「在职人员 / 离职人员」双胶囊（幂等，只做一次）
		step(h.hrMigrateRosterSubs57(ctx), "migrate HR roster subs failed: %v")
		// 第 58 轮：劳动合同字段配置拆成「劳动合同 / 劳务合同」双胶囊（幂等）
		step(h.hrMigrateContractSubs58(ctx), "migrate HR contract subs failed: %v")
		// 第 54 轮：离职样例 + 公积金网厅 87 条（均幂等，失败只告警不阻断启动）。
		step(h.hrSeedResignSamples(ctx), "seed HR resign samples failed: %v")
		step(h.hrSeedNewHireSamples(ctx), "seed HR new hire samples failed: %v")
		step(h.hrSeedFundRecords(ctx), "seed HR fund records failed: %v")
		// 第 55 轮：真实网厅附件的月度账单（2025-05 汇缴清册回单）
		step(h.hrSeedFundBills(ctx), "seed HR fund bills failed: %v")
		// 第 56 轮：242 条回盘 + 375 正常 + 3 异常 + 75 人扣款明细 + 2026-11 账单
		step(h.hrSeedSocialRound56(ctx), "seed HR social round56 failed: %v")
		// 第 61 轮：宿舍五标签样例（18 间宿舍 + 10 套公租房 + 住宿/资产/读数 + 2026-10 账单）
		step(h.hrSeedDormIfNeeded(ctx), "seed HR dorm data failed: %v")
		// 第 59 轮：首次初始化成功才置位；任一步失败则下次启动重试，不会被永久跳过。
		if initOK {
			if err := h.hrMarkInitialSeeded(ctx); err != nil {
				logger.Warnf(ctx, "mark HR initial seed failed: %v", err)
			}
		}
	}
	// 第 63 轮：花名册「合同公司 → 签约公司」归一、宿舍楼/房型修正、宿舍契约 v3
	if err := h.hrFixRosterContractCompany63(ctx); err != nil {
		logger.Warnf(ctx, "normalize roster contract company failed: %v", err)
	}
	if err := h.hrMigrateDormSubs63(ctx); err != nil {
		logger.Warnf(ctx, "migrate HR dorm subs v63 failed: %v", err)
	}
	// 第 69 轮：星号身份证还原 + 全量花名册（含离职）部门回填。
	// 两个分支都要跑：幂等、只补空值/只改打码号，不会回滚用户改动。
	h.hrRepair69(ctx)
	// 第 70 轮：公司名存量对齐、补 401–420 房源、补 12 类标准资产、回填宿舍/公租房表计。
	// 全部只补缺/只改空值，绝不触碰历史抄表与核算记录。
	h.hrRepair70(ctx)
	// 第 71 轮：持睿 11 间宿舍「表房合一」—— 房档表计对齐回原能耗真实表号（E021–E031/W021–W031）。
	// 幂等、只改宿舍档案 custom_data，不触碰任何历史抄表与核算明细。
	h.hrRepair71(ctx)
	// 第 72 轮：宿舍状态四态化（维修中→空置）、删住宿度数字段、资产档案收敛为共用字典、
	// 存量重复床位号幂等重排。全部只动配置与指定脏数据。
	h.hrRepair72(ctx)
	// 第 73 轮：能耗历史表名/表号与房源档案对齐（只改展示名，不动任何数值）、
	// 越界床位号按入住顺序重排。
	h.hrRepair73(ctx)
	// 第 75 轮：工业表计三介质恢复（电 6/水 4/气 1）、宿舍表号错配修号、
	// 「本期水费」类型改 amount、历史费用清单按单位归一口径重算。
	h.hrRepair75(ctx)
	// 第 76 轮：租户档案迁入公司设置（别名 / 编码 / 分摊方式 / 联系人）、
	// 车队保险单按交强险 / 商业险 / 险种类别重配字段与提取规则。
	h.hrRepair76(ctx)
	// 第 77 轮：宿舍/公租房档案补「表计类型」字段与存量取值、工业表计类型归一工商业。
	h.hrRepair77(ctx)
	// 第 78 轮：维保记录收敛（一般维修 / 二级维护，合格证并入二级维护）、
	// 维修结算清单三段式字段与提取规则、行驶证补「年检日期」。
	h.hrRepair78(ctx)
	if err := h.hrFixDormBuildingAndType63(ctx); err != nil {
		logger.Warnf(ctx, "fix HR dorm building/type failed: %v", err)
	}
	// 第 64 轮：费用信息胶囊重建为「月度账单汇总」15 列（幂等）
	if err := h.hrMigrateDormFeeSubs64(ctx); err != nil {
		logger.Warnf(ctx, "migrate HR dorm fee subs v64 failed: %v", err)
	}
	// 第 64 轮：补齐宿舍/公租房缺失的「所属部门」（幂等）
	if err := h.hrEnsureDormDeptSub64(ctx); err != nil {
		logger.Warnf(ctx, "ensure HR dorm dept sub failed: %v", err)
	}
	// 第 65 轮：资产档案去维修 4 字段 + 资产状态改可编辑 enum（幂等）
	if err := h.hrMigrateAssetFields65(ctx); err != nil {
		logger.Warnf(ctx, "migrate HR asset fields v65 failed: %v", err)
	}
	// 第 62 轮：宿舍字段契约 v2 + 真实房源种子 + 劳动合同签约公司归一
	if err := h.hrMigrateDormSubs62(ctx); err != nil {
		logger.Warnf(ctx, "migrate HR dorm subs v62 failed: %v", err)
	}
	if err := h.hrFixContractCompany62(ctx); err != nil {
		logger.Warnf(ctx, "normalize HR contract company failed: %v", err)
	}
	// 第 61 轮：宿舍样例数据（18 间宿舍 + 10 套公租房 + 住宿/资产/读数 + 首月账单）。
	// 独立于首次初始化门禁：老库升级后也要一次性补齐（内部按「已有记录即跳过」幂等）。
	if err := h.hrSeedDormIfNeeded(ctx); err != nil {
		logger.Warnf(ctx, "seed HR dorm data failed: %v", err)
	}
	// 第 60 轮：清理 subs 里「串组加锁状态字段」（劳动合同 2 个合同状态、
	// 正常/补退记录里的五险参保状态与账单状态）。每次启动都跑，幂等且只删不增。
	if err := h.hrRepair60Once(ctx); err != nil {
		logger.Warnf(ctx, "repair HR capsule status fields failed: %v", err)
	}
	// 第 54 轮：花名册「个人社保账号 / 个人公积金账号」升级为只读跨模块引用
	// （只补既有字段的 source / 默认表头，不会新增字段）
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
	// 第 58 轮：把花名册权威部门写回社保/公积金五张落库表（幂等，只在值变化时 UPDATE）。
	// 必须放在全部种子之后：种子里的部门是导入文件原值，要以花名册为准覆盖一次。
	if n, err := h.hrPersistDeptAuthorityAllTenants(ctx); err != nil {
		logger.Warnf(ctx, "persist HR dept authority failed: %v", err)
	} else if n > 0 {
		logger.Infof(ctx, "HR dept authority persisted on boot: %d rows", n)
	}
	// 第 58 轮：劳务人员不缴社保公积金，清掉系统自动生成的空占位（幂等，失败只告警）。
	// 必须放在全部种子之后：种子会补真实金额行，先清理反而白跑一次。
	if n, err := h.hrPurgeLaborServicePlaceholdersAllTenants(ctx); err != nil {
		logger.Warnf(ctx, "purge HR labor-service placeholders failed: %v", err)
	} else if n > 0 {
		logger.Infof(ctx, "purged HR labor-service placeholders on boot: %d rows", n)
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

// hrContractSeedSubs 第 58 轮：劳动合同 / 劳务合同双胶囊，公共字段两组各一份。
func hrContractSeedSubs() []types.FleetCategorySub {
	return hrContractSeedSubs58()
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
			// 第 58 轮：合同分类拆成「劳动合同 / 劳务合同」双胶囊，按 (group,name) 回填。
			legacy, seed = hrLegacyContractFields, hrContractSeedSubs58()
		default:
			continue
		}
		byName := make(map[string]types.FleetCategorySub, len(seed))
		byKey := make(map[string]types.FleetCategorySub, len(seed))
		rank := make(map[string]int, len(seed))
		for i, s := range seed {
			// 合同双胶囊下两组字段同名，byName 只作为「没有 group 时的兜底」，
			// 首次出现即劳动合同组，符合存量单胶囊配置的默认归属。
			if _, ok := byName[s.Name]; !ok {
				byName[s.Name] = s
			}
			byKey[s.Group+"\x00"+s.Name] = s
			if _, ok := rank[s.Name]; !ok {
				rank[s.Name] = i
			}
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
			// 双胶囊分类（花名册在职/离职、合同劳动/劳务）字段会重名，
			// 必须 (group,name) 命中优先；group 为空或旧分组名时退回 byName。
			want, ok := byKey[subs[i].Group+"\x00"+name]
			if !ok {
				want, ok = byName[name]
			}
			if ok {
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
			// 第 57 轮：在职管理 48 + 离职管理 14，两个胶囊分组
			Subs: hrRosterSeedSubs57(),
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
			// 第 56 轮：四胶囊字段契约（回盘表 28 / 正常 16 / 异常 16 / 月度账单 16）
			Subs: hrSocialSeedSubs(),
		},
		{
			Scope: hrScopeFund, Name: "公积金管理", BuiltinKey: "hr-fund", SortOrder: 1,
			Subs: hrFundSeedSubs(),
		},
		{
			// 第 61 轮：宿舍管理五胶囊（宿舍档案 / 公租房档案 / 住宿记录 / 资产档案 / 费用信息）
			Scope: hrScopeDorm, Name: "宿舍管理", BuiltinKey: "hr-dorm", SortOrder: 1,
			Subs: hrDormSeedSubs(),
		},
	}
}

// hrMigrateRosterSubs57 把花名册分类的 subs 扩展为「在职管理 + 离职管理」双胶囊。
//
// 只在存量分类还没有任何「离职管理」分组时执行一次：之后用户的列宽/默认表头/
// 拖拽顺序/自定义复制字段一律保留，绝不被启动重建覆盖。
func (h *HRHandler) hrMigrateRosterSubs57(ctx context.Context) error {
	want := hrRosterSeedSubs57()
	wantNames := make(map[string]bool, len(want))
	for _, s := range want {
		wantNames[s.Name] = true
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", hrScopeRoster, "花名册").
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		changed := false
		hasResigned := false
		for _, s := range cats[i].Subs {
			if strings.TrimSpace(s.Group) == hrRosterGroupResigned {
				hasResigned = true
				break
			}
		}
		if !hasResigned {
			next := append([]types.FleetCategorySub{}, want...)
			for _, sub := range cats[i].Subs {
				// 契约内字段以新定义为准；第 52 轮废弃字段直接丢弃；
				// 其余（用户复制出来的自定义字段、src_ 跨表引用）原样保留在末尾。
				if wantNames[sub.Name] || hrLegacyRosterFields[sub.Name] {
					continue
				}
				next = append(next, sub)
			}
			cats[i].Subs = next
			changed = true
		}
		// 兜底归一：花名册只有「在职管理 / 离职管理」两个胶囊，
		// 历史自定义字段（group 还是「在职信息」等）必须归到在职管理，
		// 否则设置抽屉会冒出第三个胶囊。
		for j := range cats[i].Subs {
			g := strings.TrimSpace(cats[i].Subs[j].Group)
			if g == hrRosterGroupActive || g == hrRosterGroupResigned {
				continue
			}
			cats[i].Subs[j].Group = hrRosterGroupActive
			cats[i].Subs[j].GroupLabel = hrRosterGroupActive
			changed = true
		}
		if !changed {
			continue
		}
		cats[i].UpdatedAt = time.Now()
		// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "花名册字段配置已归一为在职/离职双胶囊（tenant=%d）", cats[i].TenantID)
	}
	return nil
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

// seedHRCategories 幂等写 5 个分类：按 (tenant_id, scope, name) 查，存在则只
// mergeCategorySubs 补缺失字段并 Save(&row)，绝不覆盖用户顺序/宽度/启停。
//
// merge=false（第 59 轮：hr_initial_seeded 已置位）时，已存在的分类一律不动：
// mergeCategorySubs 会按字段名补回用户删掉的字段，正是「删了又复活」的根源。
func (h *HRHandler) seedHRCategories(ctx context.Context, merge bool) {
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
			if !merge {
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
	// 第 65 轮：费用明细行内编辑走 PATCH
	PATCH(string, ...gin.HandlerFunc) gin.IRoutes
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
	// 第 61 轮：无后缀预览路由（Nginx 静态正则不会劫持），前端统一走它
	r.GET("/hr/roster/resign-proof-view", h.ViewResignProof)
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
	// 第 56 轮：四标签列表（回盘表 / 正常记录 / 异常记录 / 月度账单）+ 账单明细
	r.GET("/hr/social/receipts", h.ListSIReceipts)
	r.POST("/hr/social/receipts/import", h.ImportSIReceipts)
	r.GET("/hr/social/records", h.ListSIRecords)
	r.PUT("/hr/social/records/:id", h.UpdateSIRecord)
	// 第 65 轮：正常记录 / 补退记录支持真实删除（回盘表仍不可删）
	r.DELETE("/hr/social/records/:id", h.DeleteSIRecord)
	r.POST("/hr/social/records/batch-delete", h.DeleteSIRecordsBatch)
	r.POST("/hr/social/records/import", h.ImportSIRecords)
	// 第 66 轮：导入前冲突预检（同月同险种同缴纳方已有数据 → 前端弹确认框）
	r.POST("/hr/social/records/import-check", h.CheckSIRecordsImport)
	r.GET("/hr/social/bills", h.ListSIBills)
	// 第 60 轮：生成账单前置校验（该月正常记录条数 / 是否已有账单），只读
	r.GET("/hr/social/bills/precheck", h.PrecheckSIBill)
	r.POST("/hr/social/bills", h.CreateSIBill)
	r.GET("/hr/social/bills/:id", h.GetSIBill)
	r.DELETE("/hr/social/bills/:id", h.DeleteSIBill)

	// 宿舍管理（第 61 轮：五标签）
	r.GET("/hr/dorm/records", h.ListDormRecords)
	r.POST("/hr/dorm/records", h.CreateDormRecord)
	r.PUT("/hr/dorm/records/:id", h.UpdateDormRecord)
	r.DELETE("/hr/dorm/records/:id", h.DeleteDormRecord)
	// 第 62 轮：子表专属删除路由（前端按标签调用，避免误映射到账单接口）
	r.DELETE("/hr/dorm/:module/:id", h.DeleteDormTyped)
	r.POST("/hr/dorm/records/import", h.ImportDormRecords)
	r.GET("/hr/dorm/template", h.DormTemplate)
	r.GET("/hr/dorm/stats", h.DormStats)
	r.POST("/hr/dorm/stays/transfer", h.TransferDormStays)
	r.POST("/hr/dorm/stays/checkout", h.CheckoutDormStays)
	// 第 64 轮：退宿可逆（恢复住宿，重新校验床位与性别）
	r.POST("/hr/dorm/stays/restore", h.RestoreDormStays)
	r.POST("/hr/dorm/stays/:id/restore", h.RestoreDormStay)
	r.GET("/hr/dorm/bills", h.ListDormBills)
	r.POST("/hr/dorm/bills", h.CreateDormBill)
	r.GET("/hr/dorm/bills/:id", h.GetDormBill)
	r.DELETE("/hr/dorm/bills/:id", h.DeleteDormBill)
	// 第 62 轮：费用信息 = 全员月度扣款明细（跨模块聚合）
	r.GET("/hr/dorm/charges", h.ListDormCharges)
	r.GET("/hr/dorm/charges/summary", h.hrDormChargeSummary)
	// 第 64 轮：费用信息月度账单汇总（1 月 1 行，供主列表使用）
	r.GET("/hr/dorm/charge-bills", h.ListDormChargeBills)
	r.POST("/hr/dorm/charges/generate", h.GenerateDormCharges)
	r.PUT("/hr/dorm/charges/:id", h.UpdateDormCharge)
	// 第 65 轮：费用明细行内编辑（其它扣款 / 退款 / 备注），失焦即存
	r.PATCH("/hr/dorm/charges/item/:id", h.PatchDormChargeItem)
	r.DELETE("/hr/dorm/charges/:id", h.DeleteDormCharge)

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
