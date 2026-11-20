// 第 66 轮回归：社保分险种精准导入、账单按真实记录重算、无固定期限合同冻结。
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 导入 A 险种再导入 B 险种：两者必须并存（旧实现会整月删光）。
func TestRound66SocialImportSliceKeepsOtherSchemes(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRSIRecord{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	mk := func(id, scheme string, unit float64) HRSIRecord {
		return HRSIRecord{
			ID: id, TenantID: 1, BillMonth: "2026-09", Period: "2026-09", RecordType: "normal",
			Name: "张三", IDNumber: "500101199001010011", Scheme: scheme,
			UnitAmount: unit, TotalAmount: unit, Source: "import",
		}
	}

	// 第一次导入：养老（单位侧）
	_, err := h.hrSIApplyImport66(ctx, db, 1,
		[]hrSISlice66{{Month: "2026-09", Kind: "normal", Scheme: "养老保险", Part: HRPartUnit}},
		[]HRSIRecord{mk("r1", "养老保险", 962.4)},
		map[string]map[string]bool{"500101199001010011|养老保险": {HRPartUnit: true}})
	require.NoError(t, err)

	// 第二次导入：工伤（单位侧）—— 养老必须完整保留
	_, err = h.hrSIApplyImport66(ctx, db, 1,
		[]hrSISlice66{{Month: "2026-09", Kind: "normal", Scheme: "工伤保险", Part: HRPartUnit}},
		[]HRSIRecord{mk("r2", "工伤保险", 99.25)},
		map[string]map[string]bool{"500101199001010011|工伤保险": {HRPartUnit: true}})
	require.NoError(t, err)

	var rows []HRSIRecord
	require.NoError(t, db.Where("tenant_id = ? AND bill_month = ?", 1, "2026-09").Find(&rows).Error)
	require.Len(t, rows, 2, "两个险种必须并存")
	sum := 0.0
	for _, r := range rows {
		sum += r.UnitAmount
	}
	require.InDelta(t, 962.4+99.25, sum, 0.001)
}

// 重复导入同一险种：只覆盖该切片，另一险种零损失；个人侧不被单位侧覆盖。
func TestRound66SocialImportSliceOverwriteOnlyTouched(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRSIRecord{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	// 存量：养老（单位 962.4 + 个人 481.2）、工伤（单位 99.25）
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "e1", TenantID: 1, BillMonth: "2026-09", Period: "2026-09", RecordType: "normal",
		Name: "张三", IDNumber: "500101199001010011", Scheme: "养老保险",
		UnitAmount: 962.4, PersonalAmount: 481.2, TotalAmount: 1443.6, Source: "import",
	}).Error)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "e2", TenantID: 1, BillMonth: "2026-09", Period: "2026-09", RecordType: "normal",
		Name: "张三", IDNumber: "500101199001010011", Scheme: "工伤保险",
		UnitAmount: 99.25, TotalAmount: 99.25, Source: "import",
	}).Error)

	// 冲突预检：只报「养老-单位缴纳」这一片
	conflicts := h.hrSICheckConflicts66(ctx, 1,
		[]hrSISlice66{{Month: "2026-09", Kind: "normal", Scheme: "养老保险", Part: HRPartUnit}})
	require.Len(t, conflicts, 1)
	require.Equal(t, "养老保险 - 单位缴纳", conflicts[0].Label)

	// 覆盖导入「养老-个人」：单位侧必须保留
	_, err := h.hrSIApplyImport66(ctx, db, 1,
		[]hrSISlice66{{Month: "2026-09", Kind: "normal", Scheme: "养老保险", Part: HRPartPersonal}},
		[]HRSIRecord{{
			ID: "n1", TenantID: 1, BillMonth: "2026-09", Period: "2026-09", RecordType: "normal",
			Name: "张三", IDNumber: "500101199001010011", Scheme: "养老保险",
			PersonalAmount: 500, TotalAmount: 500, Source: "import",
		}},
		map[string]map[string]bool{"500101199001010011|养老保险": {HRPartPersonal: true}})
	require.NoError(t, err)

	var pension HRSIRecord
	require.NoError(t, db.Where("scheme = ?", "养老保险").First(&pension).Error)
	require.InDelta(t, 962.4, pension.UnitAmount, 0.001, "单位侧不得被个人文件清空")
	require.InDelta(t, 500, pension.PersonalAmount, 0.001, "个人侧已覆盖")

	var injury HRSIRecord
	require.NoError(t, db.Where("scheme = ?", "工伤保险").First(&injury).Error)
	require.InDelta(t, 99.25, injury.UnitAmount, 0.001, "同月其它险种零损失")
}

// 仅 75 条工伤（合计 7,443.75）时，账单必须精确输出 7,443.75，且基数按人去重。
func TestRound66SocialBillFromRealRecords(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRSIRecord{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	for i := 0; i < 75; i++ {
		require.NoError(t, db.Create(&HRSIRecord{
			ID: "inj-" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			TenantID: 1, BillMonth: "2026-11", Period: "2026-11", RecordType: "normal",
			Name: "员工" + string(rune('A'+i%26)), IDNumber: "50010119900101" + pad2(i),
			Scheme: "工伤保险", Base: 6015, UnitAmount: 99.25, TotalAmount: 99.25, Source: "import",
		}).Error)
	}
	b := h.hrSIBillTotalsOf(ctx, 1, "2026-11")
	require.Equal(t, 75, b.Headcount)
	require.InDelta(t, 451125, b.BaseTotal, 0.01, "基数按人去重，不得按险种重复累加")
	require.InDelta(t, 7443.75, b.UnitInjury, 0.01)
	require.InDelta(t, 0, b.UnitPension, 0.001)
	require.InDelta(t, 0, b.UnitMedical, 0.001)
	require.InDelta(t, 0, b.UnitUnemployment, 0.001)
	require.InDelta(t, 7443.75, b.UnitTotal, 0.01)
	require.InDelta(t, 0, b.PersonalTotal, 0.001)
	require.InDelta(t, 7443.75, b.GrandTotal, 0.01)
}

// 同一人多险种时，基数只算一次。
func TestRound66SocialBaseCountedOncePerPerson(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRSIRecord{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}
	for i, scheme := range []string{"养老保险", "医疗保险", "工伤保险", "失业保险"} {
		require.NoError(t, db.Create(&HRSIRecord{
			ID: "m" + string(rune('0'+i)), TenantID: 1, BillMonth: "2026-08", Period: "2026-08",
			RecordType: "normal", Name: "李四", IDNumber: "500101199001010022",
			Scheme: scheme, Base: 6015, UnitAmount: 10, TotalAmount: 10, Source: "import",
		}).Error)
	}
	b := h.hrSIBillTotalsOf(ctx, 1, "2026-08")
	require.Equal(t, 1, b.Headcount)
	require.InDelta(t, 6015, b.BaseTotal, 0.01, "四个险种只能算一次基数")
}

// 无固定期限劳动合同：后端强制清空结束日期。
func TestRound66OpenEndedContractClearsEndDate(t *testing.T) {
	open := &HREmployee{CustomData: map[string]any{
		"合同类型": "无固定期限劳动合同", "合同结束日期": "2025-12-31",
		"现合同结束日期": "2025-12-31",
	}}
	hrNormalizeContractEndDate66(open)
	require.Equal(t, "", hrDormStr(open.CustomData, "合同结束日期"))
	require.Equal(t, "", hrDormStr(open.CustomData, "现合同结束日期"))

	byTerm := &HREmployee{CustomData: map[string]any{
		"现合同期限": "无固定期限", "现合同结束日期": "2025-12-31",
	}}
	hrNormalizeContractEndDate66(byTerm)
	require.Equal(t, "", hrDormStr(byTerm.CustomData, "现合同结束日期"))

	// 固定期限不动
	fixed := &HREmployee{CustomData: map[string]any{
		"合同类型": "固定期限劳动合同", "合同结束日期": "2027-12-31",
	}}
	hrNormalizeContractEndDate66(fixed)
	require.Equal(t, "2027-12-31", hrDormStr(fixed.CustomData, "合同结束日期"))
}

func pad2(i int) string {
	if i < 10 {
		return "000" + string(rune('0'+i))
	}
	if i < 100 {
		return "00" + string(rune('0'+i/10)) + string(rune('0'+i%10))
	}
	return "0" + string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))
}
