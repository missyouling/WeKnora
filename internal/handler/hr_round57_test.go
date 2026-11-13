// 第 57 / 58 轮回归：部门权威匹配与落库、账单一人多行口径、状态锁契约。
//
// 第 57 轮只做了「读取时覆盖部门」，第 58 轮把权威部门写回五张落库表。
// 本文件用 sqlite 内存库直接验证写回结果与幂等性，避免只靠真机观察。
package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func hrRound57DB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&HREmployee{}, &HRSIReceipt{}, &HRSIRecord{}, &HRSIBill{}, &HRSIBillItem{},
		&HRPFRecord{}, &HRPFBill{}, &HRPFBillItem{},
	))
	return db
}

func TestRound57DeptAuthorityIDThenName(t *testing.T) {
	db := hrRound57DB(t)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", IDNumber: "510232197708286715", Department: "技术质量部",
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e2", TenantID: 1, Name: "李四", IDNumber: "", Department: "财务部",
	}).Error)

	h := &HRHandler{db: db}
	auth := h.hrSIDeptAuthority(context.Background(), 1)

	// 身份证优先
	require.Equal(t, "技术质量部", auth.deptOf("510232197708286715", "别名"))
	// 身份证命中不到 → 回退姓名
	require.Equal(t, "技术质量部", auth.deptOf("000000000000000000", "张三"))
	require.Equal(t, "财务部", auth.deptOf("", "李四"))
	// 两者都命中不到 → 空（调用方保持原值，绝不猜）
	require.Equal(t, "", auth.deptOf("", "王五"))
	require.Equal(t, "", auth.deptOf("999999999999999999", ""))
}

func TestRound57DeptLabelAliases(t *testing.T) {
	require.Equal(t, "财务部", hrSIDeptLabel("财务"))
	require.Equal(t, "机电部", hrSIDeptLabel("机电"))
	require.Equal(t, "生管", hrSIDeptLabel("生产管理"))
	require.Equal(t, "库房", hrSIDeptLabel(" 库房 "))
	require.Equal(t, "", hrSIDeptLabel(""))
}

func TestRound57PersistDeptAuthorityWritesAllTables(t *testing.T) {
	db := hrRound57DB(t)
	ctx := context.Background()
	const idNo = "510232197708286715"
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", IDNumber: idNo, Department: "技术质量部",
	}).Error)
	// 姓名回退样本：花名册里没有身份证
	require.NoError(t, db.Create(&HREmployee{
		ID: "e2", TenantID: 1, Name: "李四", IDNumber: "", Department: "财务",
	}).Error)

	require.NoError(t, db.Create(&HRSIReceipt{
		ID: "r1", TenantID: 1, IDNumber: idNo, Name: "张三", Department: "质量部",
		CustomData: map[string]any{"部门": "质量部"},
	}).Error)
	require.NoError(t, db.Create(&HRSIReceipt{
		ID: "r2", TenantID: 1, IDNumber: "", Name: "李四", Department: "",
	}).Error)
	require.NoError(t, db.Create(&HRSIReceipt{
		ID: "r3", TenantID: 1, IDNumber: "999999999999999999", Name: "王五", Department: "旧部门",
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "n1", TenantID: 1, IDNumber: idNo, Name: "张三", Department: "质量部",
	}).Error)
	require.NoError(t, db.Create(&HRSIBillItem{
		ID: "b1", TenantID: 1, BillID: "bill-1", IDNumber: idNo, Name: "张三", Department: "质量",
	}).Error)
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "p1", TenantID: 1, IdentityNumber: idNo, Name: "张三", Department: "质量部",
		CustomData: map[string]any{"部门": "质量部"},
	}).Error)
	require.NoError(t, db.Create(&HRPFBillItem{
		ID: "pb1", TenantID: 1, BillID: "pf-bill-1", IdentityNumber: idNo, Name: "张三", Department: "质量部",
	}).Error)

	h := &HRHandler{db: db}
	res, err := h.hrPersistDeptAuthority(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 2, res.Receipts, "r1 身份证命中 + r2 姓名回退")
	require.Equal(t, 1, res.Records)
	require.Equal(t, 1, res.BillItems)
	require.Equal(t, 1, res.PFRecords)
	require.Equal(t, 1, res.PFBillItems)
	require.Equal(t, 6, res.total())

	readDept := func(model any, id string) string {
		var dept string
		row := db.Model(model).Select("department").Where("id = ?", id).Row()
		require.NoError(t, row.Scan(&dept))
		return dept
	}
	require.Equal(t, "技术质量部", readDept(&HRSIReceipt{}, "r1"))
	require.Equal(t, "财务", readDept(&HRSIReceipt{}, "r2"), "姓名回退取花名册原名")
	require.Equal(t, "旧部门", readDept(&HRSIReceipt{}, "r3"), "匹配不到绝不改")
	require.Equal(t, "技术质量部", readDept(&HRSIRecord{}, "n1"))
	require.Equal(t, "技术质量部", readDept(&HRSIBillItem{}, "b1"))
	require.Equal(t, "技术质量部", readDept(&HRPFRecord{}, "p1"))
	require.Equal(t, "技术质量部", readDept(&HRPFBillItem{}, "pb1"))

	// 幂等：值已对齐时第二次不再产生任何 UPDATE
	again, err := h.hrPersistDeptAuthority(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 0, again.total(), "重复执行必须零写入")
}

func TestRound57BillItemLabelIsIdempotent(t *testing.T) {
	db := hrRound57DB(t)
	ctx := context.Background()
	const idNo = "510232197708286715"
	// 花名册原名「财务」→ 账单明细存「财务部」；再跑一次不能变成「财务部部」
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", IDNumber: idNo, Department: "财务",
	}).Error)
	require.NoError(t, db.Create(&HRSIBillItem{
		ID: "b1", TenantID: 1, BillID: "bill-1", IDNumber: idNo, Name: "张三", Department: "财务",
	}).Error)

	h := &HRHandler{db: db}
	res, err := h.hrPersistDeptAuthority(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, res.BillItems)

	var row HRSIBillItem
	require.NoError(t, db.Where("id = ?", "b1").First(&row).Error)
	require.Equal(t, "财务部", row.Department)

	again, err := h.hrPersistDeptAuthority(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 0, again.total())
	require.NoError(t, db.Where("id = ?", "b1").First(&row).Error)
	require.Equal(t, "财务部", row.Department)
}

func TestRound57BillItemSeedKeepsChenGangTwoRows(t *testing.T) {
	var normal, adjust *hrSIBillItemSeed
	for i := range hrSIBillItemSeedRows {
		d := &hrSIBillItemSeedRows[i]
		if d.Name != "陈刚" {
			continue
		}
		if d.IsAdjustment {
			adjust = d
		} else {
			normal = d
		}
	}
	require.NotNil(t, normal, "陈刚必须有正常缴费行")
	require.NotNil(t, adjust, "陈刚必须有补缴行")
	// 两行独立存在（合并成一行会篡改金额明细）
	require.InDelta(t, 636.58, normal.PTotal, 0.001)
	require.InDelta(t, 1693.24, normal.UTotal, 0.001)
	require.InDelta(t, 511.28, adjust.PTotal, 0.001)
	require.InDelta(t, 1091.73, adjust.UTotal, 0.001)
	require.Equal(t, normal.IDNo, adjust.IDNo, "同一个人（同身份证）才构成「一人多行」")

	// 行数 76、人数 75：补缴行只加金额不加人
	items := make([]HRSIBillItem, 0, len(hrSIBillItemSeedRows))
	for _, d := range hrSIBillItemSeedRows {
		items = append(items, hrSIBillItemRowToModel(1, "bill-1", d))
	}
	require.Len(t, items, 76)
	people := 0
	for _, s := range hrSIDeptSummaryOf(items) {
		people += s["人数"].(int)
	}
	require.Equal(t, 75, people)
}

func TestRound57LockedStatusFieldsStaySingleRow(t *testing.T) {
	// 五险参保状态是系统内置加锁字段：抽屉不可编辑、设置里只能复制不能删。
	// 前端 FleetCertPanel 用单个 .fc-op 承载锁标 + ⋮（见 round57.test.ts），
	// 后端这里锁死「system 类型 + 内置 + 默认表头」的契约，避免改字段时把锁丢了。
	want := map[string]bool{
		"养老参保状态": true, "失业参保状态": true, "医疗参保状态": true,
		"工伤参保状态": true, "生育参保状态": true,
	}
	hit := map[string]bool{}
	for _, d := range hrSocialReceiptFieldDefs() {
		if !want[d.Name] {
			continue
		}
		hit[d.Name] = true
		require.Equal(t, "system", d.Type, "%s 必须是 system（前端锁标来源）", d.Name)
		require.True(t, d.Builtin, "%s 必须 builtin", d.Name)
		require.True(t, d.DefaultHeader, "%s 默认表头", d.Name)
	}
	require.Len(t, hit, len(want), "五险参保状态一个都不能少")

	// 派生到 subs 后 builtin / 默认表头必须保留（锁标记与默认列都靠它）
	for _, name := range []string{"养老参保状态", "失业参保状态", "医疗参保状态", "工伤参保状态", "生育参保状态"} {
		found := false
		for _, s := range hrSocialSeedSubs() {
			if s.Name != name {
				continue
			}
			found = true
			require.True(t, s.Builtin, name)
			require.True(t, s.IsDefault, name)
			require.Equal(t, "回盘表", s.Group, name)
		}
		require.True(t, found, name)
	}
}
