package handler

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 12 个菜单必须全部有分类配置（第 28 轮 Q2 决策）：
// 成本 7 项统一 fleet-cost scope + 7 个分类；轮胎/年检各自独立 scope。
// 缺任何一个 → 该菜单的六同步（列/筛选/抽屉/规则）无源，只能回退前端硬编码。
func TestFleetRecordCategorySeedsCoverAllMenus(t *testing.T) {
	seeds := fleetRecordCategorySeeds()
	if len(seeds) != 9 {
		t.Fatalf("应有 9 个记录型分类（轮胎 1 + 年检 1 + 成本 7），实际 %d", len(seeds))
	}
	byScope := map[string][]fleetCategorySeed{}
	for _, s := range seeds {
		if s.Scope == "" || s.Name == "" || s.BuiltinKey == "" {
			t.Fatalf("分类定义不完整: %+v", s)
		}
		if len(s.Subs) == 0 {
			t.Fatalf("%s/%s 没有默认字段", s.Scope, s.Name)
		}
		byScope[s.Scope] = append(byScope[s.Scope], s)
	}
	if len(byScope[types.FleetCategoryScopeTire]) != 1 {
		t.Fatalf("轮胎应有 1 个分类，实际 %d", len(byScope[types.FleetCategoryScopeTire]))
	}
	if len(byScope[types.FleetCategoryScopeInspection]) != 1 {
		t.Fatalf("年检应有 1 个分类，实际 %d", len(byScope[types.FleetCategoryScopeInspection]))
	}
	if len(byScope[types.FleetCategoryScopeFleetCost]) != 7 {
		t.Fatalf("成本管理应有 7 个分类，实际 %d", len(byScope[types.FleetCategoryScopeFleetCost]))
	}
}

// 字段清单必须覆盖前端旧 TYPES 的关键字段（删掉硬编码后靠它渲染表单）。
func TestFleetRecordCategorySeedsHaveKeyFields(t *testing.T) {
	// 车牌号是记录型菜单的归属键（派生台账按车牌归属、筛选器也按它派生），
	// 每个分类都必须有。
	want := map[string][]string{
		"tire":       {"车牌号", "轮胎编号", "当前状态", "安装日期"},
		"inspection": {"车牌号", "年检类型", "下次年检日期", "办理状态"},
		"加油充电":       {"车牌号", "交易类型", "油品/电价类型", "数量", "单价"},
		"路桥费":        {"车牌号", "费用类型", "通行起始日期", "通行截止日期", "含税金额", "税额"},
		"维修保养费":      {"车牌号", "维修项目", "维修类别", "工时费", "材料费"},
		"保险理赔":       {"车牌号", "理赔单号", "出险日期", "理赔状态", "定损金额"},
		"违章处理":       {"车牌号", "违章类型", "扣分", "处理状态", "缴款金额"},
		"辅材记录":       {"车牌号", "类型", "数量", "单价（元）"},
		"请车记录":       {"车牌号", "申请人", "事由", "开始时间", "审批人"},
	}
	for _, s := range fleetRecordCategorySeeds() {
		keys := make(map[string]bool, len(s.Subs))
		for _, sub := range s.Subs {
			if keys[sub.Name] {
				t.Fatalf("%s/%s 字段重复: %s", s.Scope, s.Name, sub.Name)
			}
			keys[sub.Name] = true
			switch sub.DataType {
			case "text", "textarea", "number", "date", "select", "array", "items":
			default:
				t.Fatalf("%s/%s 字段 %s 的类型不合法: %q", s.Scope, s.Name, sub.Name, sub.DataType)
			}
		}
		for _, k := range want[s.Name] {
			if !keys[k] {
				t.Fatalf("%s/%s 缺少关键字段 %q", s.Scope, s.Name, k)
			}
		}
	}
}

// 幂等合并只补缺失字段：用户在设置抽屉改过的顺序/启停/类型绝不能被 seed 覆盖。
func TestMergeCategorySubsOnlyAppendsMissing(t *testing.T) {
	existing := []types.FleetCategorySub{
		{Name: "轮胎编号", Enabled: false, IsDefault: false, DataType: "textarea", Width: 200},
	}
	expected := []types.FleetCategorySub{
		{Name: "轮胎编号", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "品牌", Enabled: true, IsDefault: false, DataType: "text"},
	}
	merged, added := mergeCategorySubs(existing, expected)
	if added != 1 {
		t.Fatalf("只应补 1 个缺失字段，实际 %d", added)
	}
	if len(merged) != 2 {
		t.Fatalf("合并后应有 2 个字段，实际 %d", len(merged))
	}
	if merged[0].DataType != "textarea" || merged[0].Enabled {
		t.Fatalf("已有字段被覆盖: %+v", merged[0])
	}
	if merged[1].Name != "品牌" || !merged[1].Enabled {
		t.Fatalf("新增字段不正确: %+v", merged[1])
	}
	// 二次合并必须零改动（幂等）
	again, added2 := mergeCategorySubs(merged, expected)
	if added2 != 0 || len(again) != 2 {
		t.Fatalf("二次合并应幂等，added=%d len=%d", added2, len(again))
	}
}
