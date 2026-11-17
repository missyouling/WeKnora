// 第 58 轮后端回归：合同类型分流、合同双胶囊字段配置、劳务人员社保公积金豁免、
// 导入模板表头动态生成。
package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

func hrRound58DB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&HREmployee{}, &HRSIRecord{}, &HRPFRecord{}, &types.FleetCategory{},
	))
	return db
}

func TestRound58ContractKind(t *testing.T) {
	// 5 种劳务类（含前后空格 / 简称包含匹配）
	for _, typ := range []string{
		"劳务合同", "返聘协议", "实习协议", "劳务派遣合同", "借调合同",
		" 劳务合同 ", "劳务派遣", "实习生协议",
	} {
		require.Equal(t, hrContractKindService, HrContractKindOf(typ), typ)
	}
	// 3 种劳动类
	for _, typ := range []string{
		"固定期限劳动合同", "无固定期限劳动合同", "以完成一定工作任务为期限的劳动合同",
	} {
		require.Equal(t, hrContractKindLabor, HrContractKindOf(typ), typ)
	}
	// 空值与无法识别的类型一律 labor
	require.Equal(t, hrContractKindLabor, HrContractKindOf(""))
	require.Equal(t, hrContractKindLabor, HrContractKindOf("   "))
	require.Equal(t, hrContractKindLabor, HrContractKindOf("其他"))
}

func TestRound58ContractSubsTwoGroups(t *testing.T) {
	subs := hrContractSeedSubs58()
	// 两组各 17 个字段，一个都不能被 (group,name) 去重吞掉
	require.Len(t, subs, 34)

	count := map[string]map[string]int{}
	for _, s := range subs {
		if count[s.Group] == nil {
			count[s.Group] = map[string]int{}
		}
		count[s.Group][s.Name]++
	}
	require.Contains(t, count, hrContractGroupLabor)
	require.Contains(t, count, hrContractGroupService)
	require.Len(t, count[hrContractGroupLabor], 17)
	require.Len(t, count[hrContractGroupService], 17)

	// 公共字段两组各一份（姓名 / 证件号码 / 工号 / 部门 + 合同 7 项）
	for _, name := range []string{
		"姓名", "证件号码", "工号", "部门",
		"合同编号", "合同类型", "合同开始日期", "合同结束日期", "合同期限", "签约公司", "签订日期",
	} {
		require.Equal(t, 1, count[hrContractGroupLabor][name], "劳动合同组缺 "+name)
		require.Equal(t, 1, count[hrContractGroupService][name], "劳务合同组缺 "+name)
	}
}

func TestRound58ContractSubsMigration(t *testing.T) {
	db := hrRound58DB(t)
	ctx := context.Background()
	// 存量单胶囊：17 个字段都没有 group，用户还把「合同编号」改名成「合同号」
	subs := hrContractSeedSubs52()
	for i := range subs {
		subs[i].Group, subs[i].GroupLabel = "", ""
		if subs[i].Name == "合同编号" {
			subs[i].Label = "合同号"
		}
	}
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "c1", TenantID: 1, Scope: hrScopeContract, Name: "劳动合同",
		Enabled: true, Subs: subs,
	}).Error)

	h := &HRHandler{db: db}
	require.NoError(t, h.hrMigrateContractSubs58(ctx))

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c1").First(&cat).Error)
	require.Len(t, cat.Subs, 34, "两组各 17 个字段")
	require.Equal(t, hrContractGroupLabor, cat.Subs[0].Group, "遗留配置默认归劳动合同")
	require.Equal(t, hrContractGroupService, cat.Subs[17].Group)
	require.Equal(t, "合同号", cat.Subs[9].Label, "用户自定义显示名必须保留")

	// 幂等：第二次执行不产生任何变化
	before := make([]types.FleetCategorySub, len(cat.Subs))
	copy(before, cat.Subs)
	require.NoError(t, h.hrMigrateContractSubs58(ctx))
	require.NoError(t, db.Where("id = ?", "c1").First(&cat).Error)
	require.Equal(t, before, cat.Subs)
}

func TestRound58LaborServiceExcludedFromAlerts(t *testing.T) {
	db := hrRound58DB(t)
	ctx := context.Background()
	// 第 59 轮：预警默认关闭，本用例验证劳务人员豁免口径，先打开开关。
	prev := hrAlertsEnabled
	hrAlertsEnabled = true
	defer func() { hrAlertsEnabled = prev }()
	// e1 / e2 劳务，e3 劳动
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", EmployeeID: "0001", Status: "active",
		IDNumber: "510232197708286711", CustomData: map[string]any{"合同类型": "劳务合同"},
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e2", TenantID: 1, Name: "李四", EmployeeID: "0002", Status: "active",
		IDNumber: "510232197708286712", CustomData: map[string]any{"合同类型": "实习协议"},
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e3", TenantID: 1, Name: "王五", EmployeeID: "0003", Status: "active",
		IDNumber: "510232197708286713", CustomData: map[string]any{"合同类型": "固定期限劳动合同"},
	}).Error)

	// 公积金：e1 空占位（该删）、e2 有金额（必留）、e3 劳动空占位（不是劳务，必留）
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "p1", TenantID: 1, Name: "张三", IdentityNumber: "510232197708286711",
		Status: hrFundStatusActive,
	}).Error)
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "p2", TenantID: 1, Name: "李四", IdentityNumber: "510232197708286712",
		PersonalBase: 5000, PersonalAmount: 300, CompanyAmount: 300, TotalAmount: 600,
		Status: hrFundStatusActive,
	}).Error)
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "p3", TenantID: 1, Name: "王五", IdentityNumber: "510232197708286713",
		Status: hrFundStatusActive,
	}).Error)

	// 社保：e1 导入空行（该删）、e2 有金额（必留）、e3 劳动空行（必留）
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "s1", TenantID: 1, Name: "张三", IDNumber: "510232197708286711",
		Source: "import", Scheme: "养老保险",
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "s2", TenantID: 1, Name: "李四", IDNumber: "510232197708286712",
		Source: "import", Scheme: "养老保险", Base: 6000, PersonalAmount: 100, UnitAmount: 200, TotalAmount: 300,
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "s3", TenantID: 1, Name: "王五", IDNumber: "510232197708286713",
		Source: "import", Scheme: "养老保险",
	}).Error)

	h := &HRHandler{db: db}

	// 待补录只剩劳动的王五
	pending := h.hrFundPendingEmployees(ctx, 1)
	require.Len(t, pending, 1)
	require.Equal(t, "王五", pending[0].Name)

	// 清理：劳务的空占位（p1 + s1）删掉，有金额 / 非劳务的行一条不动
	removed, err := h.hrPurgeLaborServicePlaceholders(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 2, removed)

	var pfIDs []string
	require.NoError(t, db.Model(&HRPFRecord{}).Order("id ASC").Pluck("id", &pfIDs).Error)
	require.Equal(t, []string{"p2", "p3"}, pfIDs)
	var siIDs []string
	require.NoError(t, db.Model(&HRSIRecord{}).Order("id ASC").Pluck("id", &siIDs).Error)
	require.Equal(t, []string{"s2", "s3"}, siIDs)

	// 幂等：再跑一次没有任何删除
	again, err := h.hrPurgeLaborServicePlaceholders(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 0, again)
}

func TestRound58DynamicTemplateHeaders(t *testing.T) {
	db := hrRound58DB(t)
	ctx := context.Background()
	cat := types.FleetCategory{
		ID: "c1", TenantID: 1, Scope: hrScopeRoster, Name: "花名册", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "姓名", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "测试扩展列", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "个人社保账号", Label: "社保账号", Enabled: true, DataType: "text",
				Group:  hrRosterGroupActive,
				Source: &types.FleetCategorySubSource{Category: "社保管理", Field: "个人社保账号", Mode: "field"}},
			{Name: "只读系统列", Enabled: true, DataType: "readonly", Group: hrRosterGroupActive},
			{Name: "系统状态列", Enabled: true, DataType: "system", Group: hrRosterGroupActive},
			{Name: "已停用列", Enabled: false, DataType: "text", Group: hrRosterGroupActive},
			{Name: "离职原因说明", Enabled: true, DataType: "textarea", Group: hrRosterGroupResigned},
		},
	}
	require.NoError(t, db.Create(&cat).Error)

	headers := hrTemplateHeadersFor(ctx, db, 1, hrScopeRoster, hrRosterGroupActive)
	require.Equal(t, []string{"工号", "姓名", "测试扩展列"}, headers,
		"只保留本胶囊内启用、非引用、非只读/系统的字段，顺序按 subs 顺序")

	// 改名（label 优先）后表头跟着变
	for i := range cat.Subs {
		if cat.Subs[i].Name == "测试扩展列" {
			cat.Subs[i].Label = "扩展列A"
		}
	}
	require.NoError(t, db.Save(&cat).Error)
	headers = hrTemplateHeadersFor(ctx, db, 1, hrScopeRoster, hrRosterGroupActive)
	require.Contains(t, headers, "扩展列A")
	require.NotContains(t, headers, "测试扩展列")

	// 删掉自定义字段后表头同步消失
	kept := make([]types.FleetCategorySub, 0, len(cat.Subs))
	for _, s := range cat.Subs {
		if s.Name == "测试扩展列" {
			continue
		}
		kept = append(kept, s)
	}
	cat.Subs = kept
	require.NoError(t, db.Save(&cat).Error)
	headers = hrTemplateHeadersFor(ctx, db, 1, hrScopeRoster, hrRosterGroupActive)
	require.Equal(t, []string{"工号", "姓名"}, headers)
	require.NotContains(t, headers, "扩展列A")

	// 离职胶囊独立取表头，且自动补三个匹配键
	resigned := hrTemplateHeadersFor(ctx, db, 1, hrScopeRoster, hrRosterGroupResigned)
	require.Equal(t, []string{"工号", "姓名", "证件号码", "离职原因说明"}, resigned)

	// 读不到分类（没有库 / 没有分类）时回退静态契约，老租户与单测都不会炸
	require.Equal(t, hrTemplateHeaders(hrModuleRoster),
		hrTemplateHeadersFor(ctx, nil, 1, hrScopeRoster, hrRosterGroupActive))
	require.Equal(t, hrTemplateHeaders(hrModuleResign),
		hrTemplateHeadersFor(ctx, db, 99, hrScopeRoster, hrRosterGroupResigned))

	// 合同胶囊：三个员工匹配键必须打头，否则整份合同导入都识别不了人
	contract := types.FleetCategory{
		ID: "c2", TenantID: 1, Scope: hrScopeContract, Name: "劳动合同", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, DataType: "readonly", Group: hrContractGroupLabor,
				Source: &types.FleetCategorySubSource{Category: "花名册", Field: "工号", Mode: "field"}},
			{Name: "合同编号", Enabled: true, DataType: "text", Group: hrContractGroupLabor},
			{Name: "合同类型", Enabled: true, DataType: "text", Group: hrContractGroupLabor},
		},
	}
	require.NoError(t, db.Create(&contract).Error)
	require.Equal(t, []string{"工号", "姓名", "证件号码", "合同编号", "合同类型"},
		hrTemplateHeadersFor(ctx, db, 1, hrScopeContract, hrContractGroupLabor))
}
