// 车队 12 个菜单的分类种子（第 28 轮 Q2/Q5 决策）。
//
// 背景：轮胎 / 年检 / 成本 7 项此前只有前端 `TYPES` / `RECORD_COLS` 两份硬编码，
// 数据库里没有任何分类 → 「字段定义 / 列表列 / 筛选器 / 抽屉字段 / 提取规则」
// 五条链路全部无法从 subs 派生，用户改一个字段要发一次前端版本。
//
// 本文件把 9 类字段一次性幂等 seed 进 fleet_categories：
// tire → 轮胎管理（单分类）；inspection → 年检管理（单分类）；
// fleet-cost → 加油充电 / 路桥费 / 维修保养费 / 保险理赔 / 违章处理 / 辅材记录 / 请车记录。
//
// 字段命名口径：**中文名即 data key**。记录型存量仅 5 条且都是档案类，没有
// `data.tire_no` 这类英文键的历史包袱，统一中文可以少一层映射
// （前端 form.data['轮胎编号'] 直存直取），也让用户在抽屉里看到的字段名
// 与 subs 完全一致。
//
// 类型只用 text/textarea/number/date：`FleetCategorySub` 没有枚举选项字段，
// 引入 select 需要额外的 options schema 与迁移；枚举值先按文本录入，
// 后续如需下拉再扩 schema（不影响已存数据）。
package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// fleetCategorySeed 一条分类种子（与库存分类按 scope+name 对齐）。
type fleetCategorySeed struct {
	Scope      string
	Name       string
	BuiltinKey string
	SortOrder  int
	Subs       []types.FleetCategorySub
}

// txt/num/dt/area 是种子字段的构造器：统一 Enabled=true、IsDefault 由调用方决定。
func txt(name string) types.FleetCategorySub {
	return types.FleetCategorySub{Name: name, Enabled: true, DataType: "text"}
}

func num(name string) types.FleetCategorySub {
	return types.FleetCategorySub{Name: name, Enabled: true, DataType: "number"}
}

func dt(name string) types.FleetCategorySub {
	return types.FleetCategorySub{Name: name, Enabled: true, DataType: "date"}
}

func area(name string) types.FleetCategorySub {
	return types.FleetCategorySub{Name: name, Enabled: true, DataType: "textarea"}
}

// def 把字段标记为「列表默认展示」。
func def(s types.FleetCategorySub) types.FleetCategorySub {
	s.IsDefault = true
	return s
}

// fleetRecordCategorySeeds 返回 9 个记录型分类的默认配置。
//
// 顺序即左侧菜单顺序：轮胎 → 年检 → 成本 7 项（加油/路桥/维修/保险/违章/辅材/请车）。
func fleetRecordCategorySeeds() []fleetCategorySeed {
	return []fleetCategorySeed{
		{
			Scope: types.FleetCategoryScopeTire, Name: "轮胎管理", BuiltinKey: "fleet-tire", SortOrder: 1,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("轮胎编号")), def(txt("品牌")), def(txt("规格型号")), txt("轮胎类型"),
				txt("适用轮位"), txt("安装轮位"), def(dt("安装日期")), num("安装里程"),
				def(txt("当前状态")), dt("购置日期"), num("购置单价"), num("当前里程"),
				txt("供应商"), txt("付款状态"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeInspection, Name: "年检管理", BuiltinKey: "fleet-inspection", SortOrder: 1,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("年检类型")), txt("检验周期"), dt("上次年检日期"), def(dt("下次年检日期")),
				num("到期提醒天数"), txt("提醒状态"), num("逾期天数"), def(txt("办理状态")),
				dt("送检日期"), txt("送检人"), txt("检测站"), def(txt("检测结果")),
				dt("检验有效期"), num("总费用"), txt("发票号"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "加油充电", BuiltinKey: "fleet-cost-fuel", SortOrder: 1,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("交易类型")), txt("消费类型"), txt("卡号"), def(txt("油品/电价类型")),
				def(num("数量")), def(num("单价")), num("百公里油耗/电耗"), txt("支付方式"),
				num("税率"), num("税额"), txt("来源发票号"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "路桥费", BuiltinKey: "fleet-cost-toll", SortOrder: 2,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("费用类型")), def(dt("通行起始日期")), def(dt("通行截止日期")),
				txt("入口站/起点"), txt("出口站/终点"), txt("停车场名称"),
				num("税率"), def(num("税额")), def(num("含税金额")), num("停车费"),
				txt("支付方式"), txt("票据号码"), txt("来源发票号"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "维修保养费", BuiltinKey: "fleet-cost-repair", SortOrder: 3,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("维修项目")), def(txt("维修类别")), txt("维修厂/供应商"), txt("工单号"),
				def(num("工时费")), def(num("材料费")), txt("发票号"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "保险理赔", BuiltinKey: "fleet-cost-claim", SortOrder: 4,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("理赔单号")), txt("关联保单号"), txt("保险公司"), txt("险种"),
				def(dt("出险日期")), dt("报案日期"), txt("司机姓名"), txt("事故类型"),
				txt("事故地点"), txt("事故责任"), def(num("定损金额")), dt("赔付日期"),
				txt("赔付方式"), def(txt("理赔状态")), num("免赔金额"), num("自付金额"),
				dt("结案日期"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "违章处理", BuiltinKey: "fleet-cost-violation", SortOrder: 5,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("违章单号")), txt("司机姓名"), def(txt("违章类型")), txt("违章行为"),
				txt("违章地点"), txt("违章代码"), def(num("扣分")), txt("处罚决定书号"),
				def(txt("处理状态")), txt("处理人"), dt("处理日期"), dt("缴款日期"),
				num("缴款金额"), txt("缴款方式"), txt("是否已缴款"), txt("责任人"),
				txt("承担方"), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "辅材记录", BuiltinKey: "fleet-cost-material", SortOrder: 6,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("类型")), def(num("数量")), def(num("单价（元）")), area("备注"),
			},
		},
		{
			Scope: types.FleetCategoryScopeFleetCost, Name: "请车记录", BuiltinKey: "fleet-cost-request", SortOrder: 7,
			Subs: []types.FleetCategorySub{
				def(txt("车牌号")), def(txt("申请人")), def(area("事由")), def(dt("开始时间")), dt("结束时间"),
				txt("审批人"), area("备注"),
			},
		},
	}
}

// mergeCategorySubs 把 expected 中**缺失**的字段追加到 existing，返回新数组与新增数。
//
// 只补不改：用户在设置抽屉里调整过的顺序、启停、类型、宽度一律保留；
// 同名即视为同一字段（中文名就是 data key，改名等同新增字段，属预期行为）。
func mergeCategorySubs(existing, expected []types.FleetCategorySub) ([]types.FleetCategorySub, int) {
	have := make(map[string]bool, len(existing))
	for _, s := range existing {
		have[s.Name] = true
	}
	merged := existing
	added := 0
	for _, e := range expected {
		if have[e.Name] {
			continue
		}
		merged = append(merged, e)
		have[e.Name] = true
		added++
	}
	return merged, added
}

// seedFleetRecordCategories 幂等写入 9 个记录型分类。
//
// 只处理 tenant_id=0 的模板行（与发票分类 seed 同口径）：用户租户的分类由
// 「从模板复制」链路创建，直接在模板上补字段即可被后续新建租户继承。
// 存量租户若已有同名分类，这里也会按 (tenant_id, scope, name) 命中并补字段。
func (h *FleetHandler) seedFleetRecordCategories(ctx context.Context) {
	// 记录型 scope 按租户隔离（与 vehicle/driver/maintain 同口径），
	// 因此要为「已有分类的每个租户」各补一份，而不是只写 tenant_id=0 的模板行 ——
	// 前端 ListFleetCategories 走 tenant_id = 当前租户，模板行查不到。
	tenantIDs := []int64{0}
	var existing []int64
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("deleted_at IS NULL AND tenant_id > 0").
		Distinct().Pluck("tenant_id", &existing).Error; err == nil {
		tenantIDs = append(tenantIDs, existing...)
	}
	for _, tenantID := range tenantIDs {
		h.seedFleetRecordCategoriesForTenant(ctx, tenantID)
	}
}

// seedFleetRecordCategoriesForTenant 为单个租户补齐 9 个记录型分类。
func (h *FleetHandler) seedFleetRecordCategoriesForTenant(ctx context.Context, tenantID int64) {
	for _, seed := range fleetRecordCategorySeeds() {
		var rows []types.FleetCategory
		err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL",
				tenantID, seed.Scope, seed.Name).
			Find(&rows).Error
		if err != nil {
			logger.Warnf(ctx, "seed fleet record category %s/%s: query failed: %v", seed.Scope, seed.Name, err)
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
				logger.Warnf(ctx, "seed fleet record category %s/%s: create failed: %v", seed.Scope, seed.Name, cerr)
				continue
			}
			logger.Infof(ctx, "seeded fleet record category %s/%s (fields=%d)", seed.Scope, seed.Name, len(seed.Subs))
			continue
		}
		for i := range rows {
			merged, added := mergeCategorySubs(rows[i].Subs, seed.Subs)
			if added == 0 {
				continue
			}
			// 必须用结构体 Save，不能 Update("subs", 切片)：GORM 会把 []FleetCategorySub
			// 当 record 传给 PG，报 `column "subs" is of type jsonb but expression is
			// of type record`（SQLSTATE 42804），字段永远补不进去。Save 走 JSON 序列化，
			// PG 与 sqlite 都能落库。
			rows[i].Subs = merged
			if uerr := h.db.WithContext(ctx).Save(&rows[i]).Error; uerr != nil {
				logger.Warnf(ctx, "seed fleet record category %s/%s: backfill failed: %v",
					seed.Scope, seed.Name, uerr)
				continue
			}
			logger.Infof(ctx, "backfilled %d field(s) into fleet category %s/%s", added, seed.Scope, seed.Name)
		}
	}
}
