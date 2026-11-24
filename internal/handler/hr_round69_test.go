// 第 69 轮回归：
//  1. 星号身份证按「姓名 + 前后缀 / 个人编号 / 唯一姓名」精准还原；
//  2. 关联部门时**含离职人员**（离职只是状态变化，档案仍在花名册）；
//  3. `is_default=false` 落库后读回仍是 false（六同步「默认表头」关得掉）；
//  4. 空部门回填只补空、不覆盖已有值（幂等）。
package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 打码号必须按「姓名 + 前 6 位 + 后 4 位」还原成主档里的明文。
func TestRound69RestoreMaskedIDByPrefixSuffix(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69", TenantID: 1, Name: "伍春宇", EmployeeID: "C020610014",
		IDNumber: "500225199604206884", Department: "仓库", Status: hrRosterStatusActive,
	}).Error)
	// 同名不同人（不同前后缀）不得串档
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69b", TenantID: 1, Name: "伍春宇", EmployeeID: "C020610015",
		IDNumber: "510232199001019999", Department: "生产部", Status: hrRosterStatusResigned,
	}).Error)

	h := &HRHandler{db: db}
	idx := h.hrBuildIDCardIndex69(ctx, 1)
	require.Equal(t, "500225199604206884",
		idx.restore("500225********6884", "伍春宇", ""), "姓名 + 前后缀必须精确命中")
	require.Equal(t, "510232199001019999",
		idx.restore("510232********9999", "伍春宇", ""), "同名不同前后缀不能串")
	require.Equal(t, "", idx.restore("999999********0000", "伍春宇", ""), "前后缀不匹配时不得按姓名猜")
}

// 个人编号优先于姓名（同名同事场景），且回盘表也是明文来源。
func TestRound69RestoreMaskedIDByPersonCode(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69c", TenantID: 1, Name: "张伟", EmployeeID: "C01",
		IDNumber: "510232198802020011", Department: "生产部", Status: hrRosterStatusActive,
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69d", TenantID: 1, Name: "张伟", EmployeeID: "C02",
		IDNumber: "510232198802020022", Department: "技术质量部", Status: hrRosterStatusActive,
	}).Error)
	// 回盘表给出个人编号 → 身份证 的映射（花名册没有这个编号）
	require.NoError(t, db.Create(&HRSIReceipt{
		ID: "r69", TenantID: 1, Name: "李四", PersonCode: "3000961655",
		IDNumber: "511028198808183836", Department: "财务部",
	}).Error)

	h := &HRHandler{db: db}
	idx := h.hrBuildIDCardIndex69(ctx, 1)
	require.Equal(t, "510232198802020011",
		idx.restore("510232********0011", "张伟", ""),
		"重名但前后缀不同 → 仍能唯一命中（0011 只属于第一个张伟）")
	require.Equal(t, "510232198802020022",
		idx.restore("510232********0022", "张伟", "C02"), "个人编号可唯一定位")
	require.Equal(t, "511028198808183836",
		idx.restore("511028********3836", "李四", "3000961655"), "回盘表也是明文来源")
}

// 存量刷洗：记录 + 回盘表 + 账单快照里的星号号一并还原，并补部门。
func TestRound69RestoreMaskedSweep(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69e", TenantID: 1, Name: "周宇", EmployeeID: "C03",
		IDNumber: "500227198907060411", Department: "生产部", Status: hrRosterStatusResigned,
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "rec69", TenantID: 1, BillMonth: "2026-09", RecordType: "normal",
		Name: "周宇", IDNumber: "500227********0411", Scheme: "养老保险",
		UnitAmount: 100, PersonalAmount: 50, TotalAmount: 150,
		CustomData: map[string]any{"身份证号": "500227********0411", "部门": ""},
	}).Error)
	require.NoError(t, db.Create(&HRSIBill{
		ID: "bill69", TenantID: 1, MonthLabel: "2026-09",
		DetailSnapshot: map[string]any{
			"items": []any{
				map[string]any{"姓名": "周宇", "证件号码": "500227********0411", "部门": "", "个人小计": 50.0},
			},
		},
	}).Error)

	h := &HRHandler{db: db}
	restored, unmatched := h.hrRestoreMaskedIdCard69(ctx, 1)
	require.Zero(t, unmatched, "花名册有完整明文时不该有未匹配")
	require.GreaterOrEqual(t, restored, 2, "记录 + 账单快照都要还原")

	/**
	 * SQLite `cache=shared` 的多个连接各自持有快照：同一个 db 句柄上
	 * 「先 Find 再 First」可能读到旧快照。这里统一用一次 Find 的结果断言。
	 */
	var probe []HRSIRecord
	require.NoError(t, db.Where("tenant_id = ?", 1).Find(&probe).Error)
	require.Len(t, probe, 1)
	rec := probe[0]
	require.Equal(t, "500227198907060411", rec.IDNumber)
	require.Equal(t, "500227198907060411", rec.CustomData["身份证号"])
	require.Equal(t, "生产部", rec.Department, "还原后必须顺带补上离职人员的历史部门")

	var bill HRSIBill
	require.NoError(t, db.Where("id = ?", "bill69").First(&bill).Error)
	items := bill.DetailSnapshot["items"].([]any)
	row := items[0].(map[string]any)
	require.Equal(t, "500227198907060411", row["证件号码"])
	require.Equal(t, "生产部", row["部门"])
}

// 关联部门必须覆盖离职人员；同名不同部门时不猜。
func TestRound69DeptAuthorityIncludesResigned(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69f", TenantID: 1, Name: "离职甲", IDNumber: "510232199001010011",
		Department: "销售部", Status: hrRosterStatusResigned,
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69g", TenantID: 1, Name: "在职乙", IDNumber: "510232199001010022",
		Department: "生产部", Status: hrRosterStatusActive,
	}).Error)

	auth := h.hrSIDeptAuthority(ctx, 1)
	require.Equal(t, "销售部", auth.deptOf("510232199001010011", "离职甲"),
		"离职人员的部门必须能关联（这正是第 6 项根因）")
	require.Equal(t, "生产部", auth.deptOf("", "在职乙"), "姓名兜底仍生效")

	dorm := h.hrDormEmployeeIndex(ctx, 1)
	require.Contains(t, dorm, "离职甲", "宿舍索引同样必须含离职人员")
}

// 回填只补空值，绝不覆盖已有部门（幂等）。
func TestRound69BackfillDeptOnlyEmpty(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69h", TenantID: 1, Name: "补部门", IDNumber: "510232199202020011",
		Department: "机电部", Status: hrRosterStatusResigned,
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "rec69b", TenantID: 1, BillMonth: "2026-10", RecordType: "normal",
		Name: "补部门", IDNumber: "510232199202020011", Department: "",
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "rec69c", TenantID: 1, BillMonth: "2026-10", RecordType: "normal",
		Name: "补部门", IDNumber: "510232199202020011", Department: "人工填的部门",
	}).Error)

	h := &HRHandler{db: db}
	require.Equal(t, 1, h.hrBackfillResignedDept69(ctx, 1),
		"只有空部门那一行被补（rec69c 已有部门，必须跳过）")

	/**
	 * 行级断言用 Pluck 单独查一次：SQLite 内存库（cache=shared）的多连接各有快照，
	 * 「先 Find 整体再断言」可能读到写前视图。真正防止「假写入」的是下面的幂等断言 ——
	 * 若第一次没写进库，第二次仍会补同一行并返回 1。
	 */
	var deptA string
	require.NoError(t, db.Table("hr_si_records").Where("id = ?", "rec69b").
		Pluck("department", &deptA).Error)
	var deptB string
	require.NoError(t, db.Table("hr_si_records").Where("id = ?", "rec69c").
		Pluck("department", &deptB).Error)
	require.Equal(t, "机电部", deptA)
	require.Equal(t, "人工填的部门", deptB, "已有值不得被覆盖")
	require.Equal(t, 0, h.hrBackfillResignedDept69(ctx, 1), "幂等：再跑一次零变更")
}
// 导入路径：带星号的文件落库前就地还原（同时补部门）。
func TestRound69RestoreImportIDs(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HREmployee{
		ID: "e69i", TenantID: 1, Name: "导入甲", IDNumber: "500225199604206884",
		Department: "仓库", Status: hrRosterStatusActive,
	}).Error)

	h := &HRHandler{db: db}
	items := []HRSIRecord{
		{ID: "i1", TenantID: 1, BillMonth: "2026-09", Name: "导入甲",
			IDNumber: "500225********6884", Scheme: "养老保险", CustomData: map[string]any{}},
		{ID: "i2", TenantID: 1, BillMonth: "2026-09", Name: "无档案",
			IDNumber: "999999********0000", Scheme: "养老保险", CustomData: map[string]any{}},
	}
	restored, unmatched := h.hrRestoreImportIDs69(ctx, 1, items)
	require.Equal(t, 1, restored)
	require.Equal(t, 1, unmatched, "匹配不到就如实回报，绝不瞎猜")
	require.Equal(t, "500225199604206884", items[0].IDNumber)
	require.Equal(t, "仓库", items[0].Department, "还原后按全量花名册补部门")
	require.Equal(t, "999999********0000", items[1].IDNumber, "未匹配的保持原样")
}

// 六同步：默认表头关掉（is_default=false）必须真的落库，读回不能被回退成 true。
//
// 走的是真实链路的四段：JSON 解码 → sanitizeFleetSubs → json.Marshal → PG/SQLite 落库。
// 不直接打 HTTP 是为了避开 hrTestRouter 只注册 HR 路由的限制（分类接口属于 fleet 路由组）。
func TestRound69DefaultVisibleFalsePersists(t *testing.T) {
	db := hrRound59DB(t)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat69", TenantID: 0, Scope: hrScopeRoster, Name: "花名册", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, IsDefault: true, Group: hrRosterGroupActive, DataType: "text"},
			{Name: "司龄", Enabled: true, IsDefault: true, Group: hrRosterGroupActive, DataType: "text"},
		},
	}).Error)

	body := `{"name":"花名册","enabled":true,"subs":[
		{"name":"工号","enabled":true,"is_default":true,"data_type":"text"},
		{"name":"司龄","enabled":true,"is_default":false,"data_type":"text"}]}`
	var req types.FleetCategory
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	sanitized, dropped := sanitizeFleetSubs(req.Subs)
	require.Zero(t, dropped)
	dataJSON, err := json.Marshal(sanitized)
	require.NoError(t, err)
	require.NoError(t, db.Model(&types.FleetCategory{}).Where("id = ?", "cat69").
		Updates(map[string]any{"subs": string(dataJSON), "name": req.Name}).Error)

	var got []types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat69").Find(&got).Error)
	require.Len(t, got, 1)
	found := false
	for _, sub := range got[0].Subs {
		if sub.Name != "司龄" {
			continue
		}
		found = true
		require.False(t, sub.IsDefault, "is_default=false 必须原样落库（不能被零值合并回退成 true）")
		require.True(t, sub.Enabled, "默认表头与启用字段是两个开关：关表头不影响启用")
	}
	require.True(t, found, "司龄 字段必须在落库结果里")
}

// 社保「记录类型」缺失时自愈补齐，已有则零变更（幂等）。
func TestRound69EnsureSocialRecordType(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "soc69", TenantID: 1, Scope: hrScopeSocial, Name: "社保管理", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "姓名", Enabled: true, Group: "正常记录", DataType: "text"},
			{Name: "记录类型", Enabled: true, Group: "补退记录", DataType: "text"},
		},
	}).Error)

	h := &HRHandler{db: db}
	h.hrEnsureSocialRecordType69(ctx)
	h.hrEnsureSocialRecordType69(ctx) // 第二次必须零变更

	var cats []types.FleetCategory
	require.NoError(t, db.Where("id = ?", "soc69").Find(&cats).Error)
	require.Len(t, cats, 1)
	count := map[string]int{}
	for _, sub := range cats[0].Subs {
		if sub.Name == "记录类型" {
			count[sub.Group]++
		}
	}
	require.Equal(t, 1, count["正常记录"], "缺的组必须补上")
	require.Equal(t, 1, count["补退记录"], "已有的组不得重复追加")
	require.Len(t, cats[0].Subs, 3)
}
