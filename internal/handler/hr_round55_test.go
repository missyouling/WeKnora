// 第 55 轮回归：真实网厅 87 条 + 2025.05 汇缴清册 18 人、双胶囊契约、全员联动预警。
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRound55RealWebhallSeed(t *testing.T) {
	require.Len(t, hrWebhallRows, 87)
	counts := map[string]int{}
	accounts := map[string]string{}
	for _, r := range hrWebhallRows {
		counts[r.status]++
		assert.True(t, hrLooksLikeFundAccount(r.account), "%s 账号必须是 12 位纯数字", r.name)
		accounts[r.name] = r.account
	}
	assert.Equal(t, 74, counts[hrFundStatusActive])
	assert.Equal(t, 7, counts[hrFundStatusSealed])
	assert.Equal(t, 6, counts[hrFundStatusExtracted])
	// 任务给定的三个真实账号（与《单位职工明细-网厅.xls》逐行一致）
	assert.Equal(t, "801504241502", accounts["寇江"])
	assert.Equal(t, "801364932880", accounts["李永娇"])
	assert.Equal(t, "801507495053", accounts["贺伟"])

	// 18 字段全量落库：余额 / 银行 / 卡号 / 四个「是否」都来自真实文件
	li := hrWebhallRows[0]
	rec := hrFundRecordOfSeed(li, 10000)
	hrFundSyncCustom(&rec)
	assert.Equal(t, li.balance, toFloat(toStringValue(rec.CustomData["账户余额(元)"])))
	assert.Equal(t, li.bank, toStringValue(rec.CustomData["联名卡银行"]))
	assert.Equal(t, li.card, toStringValue(rec.CustomData["联名卡号"]))
	assert.Equal(t, li.unitRatio, toFloat(toStringValue(rec.CustomData["单位缴存比例(%)"])))
	assert.Equal(t, "正常", toStringValue(rec.CustomData["账户状态"]))
}

func TestRound55RealHuijiaoBill(t *testing.T) {
	require.Len(t, hrHuijiao202505Rows, 18)
	var base, personal, unit, total float64
	for _, r := range hrHuijiao202505Rows {
		base += r.base
		personal += r.personal
		unit += r.unit
		total += r.total
	}
	// 回单合计：月应缴 5,680 / 个人 2,840 / 单位 2,840
	assert.Equal(t, 5680.0, total)
	assert.Equal(t, 2840.0, personal)
	assert.Equal(t, 2840.0, unit)
	assert.Equal(t, 47280.0, base)
}

func TestRound55FundDualCapsuleContract(t *testing.T) {
	assert.Len(t, hrFundFieldDefs(), 18)
	assert.Len(t, hrFundBillFieldDefs(), 8)
	subs := hrFundSeedSubs()
	assert.Len(t, subs, 26)
	bill, detail := 0, 0
	for _, s := range subs {
		switch s.Group {
		case "月度账单":
			bill++
		case "职工明细":
			detail++
		}
	}
	assert.Equal(t, 8, bill)
	assert.Equal(t, 18, detail)
	// 4 个「是否」默认表头关闭
	for _, name := range []string{"是否公贷", "是否冻结", "是否可在线提取公贷", "是否可在线提取商贷"} {
		for _, s := range subs {
			if s.Name == name {
				assert.False(t, s.IsDefault, name)
			}
		}
	}
	assert.Equal(t, []string{"已汇缴", "待汇缴", "已作废"}, hrEnumOptions("账单状态"))
}

func TestRound55FundAlerts(t *testing.T) {
	db := hrTestDB(t)
	h := &HRHandler{db: db}
	ctx := context.Background()
	const tid = int64(0)

	// 1 在职有账号+基数（正常）；1 在职无账号（待补录）；1 在职有账号但基数 0（待补录）
	require.NoError(t, db.Create(&HREmployee{
		ID: "a1", TenantID: tid, Name: "在职甲", IDNumber: "510232198001010011", Status: "在职",
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "a2", TenantID: tid, Name: "在职乙", IDNumber: "510232198001010012", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "a3", TenantID: tid, Name: "在职丙", IDNumber: "510232198001010013", Status: "在职",
	}).Error)
	// 3 离职：账户仍正常 / 已封存 / 尚未建户
	require.NoError(t, db.Create(&HREmployee{
		ID: "r1", TenantID: tid, Name: "离职甲", IDNumber: "510232198001010021", Status: "离职",
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "r2", TenantID: tid, Name: "离职乙", IDNumber: "510232198001010022", Status: "resigned",
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "r3", TenantID: tid, Name: "离职丙", IDNumber: "510232198001010023", Status: "离职",
	}).Error)

	require.NoError(t, db.Create(&HRPFRecord{
		ID: "f1", TenantID: tid, PersonalAccount: "801000000001", Name: "在职甲",
		IdentityNumber: "510232198001010011", PersonalBase: 5000, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "f2", TenantID: tid, PersonalAccount: "801000000003", Name: "在职丙",
		IdentityNumber: "510232198001010013", PersonalBase: 0, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "f3", TenantID: tid, PersonalAccount: "801000000021", Name: "离职甲",
		IdentityNumber: "510232198001010021", PersonalBase: 5000, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&HRPFRecord{
		ID: "f4", TenantID: tid, PersonalAccount: "801000000022", Name: "离职乙",
		IdentityNumber: "510232198001010022", PersonalBase: 5000, Status: "sealed",
	}).Error)

	alerts := h.hrFundAlerts(ctx, tid)
	assert.Equal(t, 2, alerts.pending, "无账号 + 基数 0 都应提醒补录")
	assert.Equal(t, 2, alerts.toSeal, "账户正常 + 未建户的离职员工都应提醒封存")
	assert.Len(t, h.hrFundPendingEmployees(ctx, tid), 2)
	assert.Len(t, h.hrFundToSealEmployees(ctx, tid), 2)
}
