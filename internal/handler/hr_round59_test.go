// 第 59 轮回归：初始化只做一次（删改不复活）、胶囊隔离、分组更名、预警关闭。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

func hrRound59DB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.FleetCategory{}, &types.SystemSetting{},
		&HREmployee{}, &HRSIPeriod{}, &HRSIRawRecord{}, &HRSICharge{}, &HRSIChange{},
		&HRPFSettings{}, &HRPFRecord{}, &HRPFBill{}, &HRPFBillItem{},
		&HRSIReceipt{}, &HRSIRecord{}, &HRSIBill{}, &HRSIBillItem{},
	))
	return db
}

// 第 59 轮：首次初始化后写 hr_initial_seeded；之后重启不回填用户删掉的字段 / 记录。
func TestRound59InitialSeededNoResurrection(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()

	// 先放一个在职员工，让业务种子（离职样例 / 公积金 / 社保）有租户可落。
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "在职甲", EmployeeID: "0001",
		IDNumber: "510232198001010011", Status: hrRosterStatusActive,
	}).Error)

	NewHRHandler(db)
	require.True(t, (&HRHandler{db: db}).hrInitialSeeded(ctx), "首次初始化后必须置位 hr_initial_seeded")

	// 用户动作：删掉花名册一个字段、删掉劳动合同组的「合同状态」、删掉一条离职样例、删掉一条公积金记录。
	var roster types.FleetCategory
	require.NoError(t, db.Where("scope = ? AND deleted_at IS NULL", hrScopeRoster).First(&roster).Error)
	kept := make([]types.FleetCategorySub, 0, len(roster.Subs))
	for _, s := range roster.Subs {
		if s.Name == "民族" {
			continue
		}
		kept = append(kept, s)
	}
	roster.Subs = kept
	require.NoError(t, db.Save(&roster).Error)

	var contract types.FleetCategory
	require.NoError(t, db.Where("scope = ? AND deleted_at IS NULL", hrScopeContract).First(&contract).Error)
	contractSubs := make([]types.FleetCategorySub, 0, len(contract.Subs))
	removedStatus := false
	for _, s := range contract.Subs {
		if s.Group == hrContractGroupLabor && s.Name == "合同状态" {
			removedStatus = true
			continue
		}
		contractSubs = append(contractSubs, s)
	}
	require.True(t, removedStatus, "劳动合同组必须有「合同状态」")
	contract.Subs = contractSubs
	require.NoError(t, db.Save(&contract).Error)

	var resigned HREmployee
	require.NoError(t, db.Where("tenant_id = ? AND name = ?", 1, "王建国").First(&resigned).Error)
	require.NoError(t, db.Delete(&resigned).Error)

	var pf HRPFRecord
	require.NoError(t, db.Where("tenant_id = ?", 1).First(&pf).Error)
	require.NoError(t, db.Where("id = ?", pf.ID).Delete(&HRPFRecord{}).Error)

	var fundBefore, socialBefore int64
	require.NoError(t, db.Model(&HRPFRecord{}).Where("tenant_id = ?", 1).Count(&fundBefore).Error)
	require.NoError(t, db.Model(&HRSIRecord{}).Where("tenant_id = ?", 1).Count(&socialBefore).Error)

	// 第二次启动：只做幂等修复，已删除的字段与记录不得复活。
	NewHRHandler(db)

	var again types.FleetCategory
	require.NoError(t, db.Where("id = ?", roster.ID).First(&again).Error)
	for _, s := range again.Subs {
		require.NotEqual(t, "民族", s.Name, "用户删掉的字段不得被启动种子加回来")
	}
	var contractAgain types.FleetCategory
	require.NoError(t, db.Where("id = ?", contract.ID).First(&contractAgain).Error)
	for _, s := range contractAgain.Subs {
		require.False(t, s.Group == hrContractGroupLabor && s.Name == "合同状态",
			"用户删掉的合同状态不得被迁移加回来（安全修复只去重、不新增）")
	}

	var n int64
	require.NoError(t, db.Model(&HREmployee{}).Where("tenant_id = ? AND name = ?", 1, "王建国").Count(&n).Error)
	require.Equal(t, int64(0), n, "用户删掉的业务记录不得复活")

	var fundAfter, socialAfter int64
	require.NoError(t, db.Model(&HRPFRecord{}).Where("tenant_id = ?", 1).Count(&fundAfter).Error)
	require.NoError(t, db.Model(&HRSIRecord{}).Where("tenant_id = ?", 1).Count(&socialAfter).Error)
	require.Equal(t, fundBefore, fundAfter, "公积金记录数不得因重启变化")
	require.Equal(t, socialBefore, socialAfter, "社保记录数不得因重启变化")

	// 标记行落在已有的 system_settings 表里，值类型 bool。
	var row types.SystemSetting
	require.NoError(t, db.Where("key = ?", hrInitialSeededKey).First(&row).Error)
	require.Equal(t, "bool", row.ValueType)
	ok, err := row.AsBool()
	require.NoError(t, err)
	require.True(t, ok)

	// 幂等：第三次启动不报错、不产生新字段。
	NewHRHandler(db)
	var finalRoster types.FleetCategory
	require.NoError(t, db.Where("id = ?", roster.ID).First(&finalRoster).Error)
	require.Len(t, finalRoster.Subs, len(roster.Subs))
}

// 第 59 轮：安全修复只清重复「合同状态」与串入正常/补退胶囊的参保状态字段。
func TestRound59Repair58Once(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "s1", TenantID: 1, Scope: hrScopeSocial, Name: "社保管理", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "个人编号", Group: hrSocReceiptGroup, GroupLabel: hrSocReceiptGroup, Enabled: true},
			{Name: "养老参保状态", Group: hrSocReceiptGroup, GroupLabel: hrSocReceiptGroup, Enabled: true},
			{Name: "账单月份", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "养老参保状态", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "医疗参保状态", Group: hrSocNormalGroup, GroupLabel: hrSocNormalGroup, Enabled: true},
			{Name: "账单月份", Group: hrSocAbnormalGroup, GroupLabel: hrSocAbnormalGroup, Enabled: true},
			{Name: "工伤参保状态", Group: hrSocAbnormalGroup, GroupLabel: hrSocAbnormalGroup, Enabled: true},
		},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "c1", TenantID: 1, Scope: hrScopeContract, Name: "劳动合同", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "合同编号", Group: hrContractGroupLabor, GroupLabel: hrContractGroupLabor, Enabled: true},
			{Name: "合同状态", Group: hrContractGroupLabor, GroupLabel: hrContractGroupLabor, Enabled: true, Builtin: true},
			{Name: "合同状态", Group: hrContractGroupLabor, GroupLabel: hrContractGroupLabor, Enabled: true},
			{Name: "合同状态", Group: hrContractGroupService, GroupLabel: hrContractGroupService, Enabled: true, Builtin: true},
		},
	}).Error)

	h := &HRHandler{db: db}
	require.NoError(t, h.hrRepair58Once(ctx))

	var social types.FleetCategory
	require.NoError(t, db.Where("id = ?", "s1").First(&social).Error)
	require.Len(t, social.Subs, 4, "正常 / 补退胶囊里的参保状态必须清掉，回盘表保留")
	byGroup := map[string][]string{}
	for _, s := range social.Subs {
		byGroup[s.Group] = append(byGroup[s.Group], s.Name)
	}
	require.Equal(t, []string{"个人编号", "养老参保状态"}, byGroup[hrSocReceiptGroup])
	require.Equal(t, []string{"账单月份"}, byGroup[hrSocNormalGroup])
	require.Equal(t, []string{"账单月份"}, byGroup[hrSocAbnormalGroup])

	var contract types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c1").First(&contract).Error)
	count := map[string]int{}
	for _, s := range contract.Subs {
		if s.Name == "合同状态" {
			count[s.Group]++
		}
	}
	require.Equal(t, 1, count[hrContractGroupLabor], "同一胶囊只留第一条合同状态")
	require.Equal(t, 1, count[hrContractGroupService])
	require.Len(t, contract.Subs, 3)

	// 幂等：第二次不再改动。
	before := append([]types.FleetCategorySub{}, contract.Subs...)
	require.NoError(t, h.hrRepair58Once(ctx))
	require.NoError(t, db.Where("id = ?", "c1").First(&contract).Error)
	require.Equal(t, before, contract.Subs)
}

// 第 59 轮：社保四胶囊隔离 —— 五险参保状态只属于回盘表。
func TestRound59SocialCapsuleIsolation(t *testing.T) {
	subs := hrSocialSeedSubs()
	require.Len(t, subs, 76, "28 + 16 + 16 + 16")

	statusCount := map[string]int{}
	for _, s := range subs {
		if hrIsSIStatusField(s.Name) {
			statusCount[s.Group]++
			require.Equal(t, hrSocReceiptGroup, s.Group, "参保状态字段只能出现在回盘表胶囊")
		}
		if s.Group == hrSocNormalGroup || s.Group == hrSocAbnormalGroup {
			require.NotContains(t, hrSIStatusFields, s.Name, s.Group+" 胶囊不得包含参保状态字段")
		}
	}
	require.Len(t, statusCount, 1)
	require.Equal(t, 5, statusCount[hrSocReceiptGroup])

	for _, group := range []string{hrSocNormalGroup, hrSocAbnormalGroup} {
		defs := hrSocialRecordFieldDefs(group)
		require.Len(t, defs, 16, group)
		for _, d := range defs {
			require.False(t, hrIsSIStatusField(d.Name), group+" 派生字段不得包含 "+d.Name)
		}
	}
}

// 第 59 轮：劳动合同 / 劳务合同每组恰好 1 个内置「合同状态」。
func TestRound59ContractOneStatusPerGroup(t *testing.T) {
	subs := hrContractSeedSubs58()
	require.Len(t, subs, 34)
	count := map[string]int{}
	for _, s := range subs {
		if s.Name == "合同状态" {
			count[s.Group]++
			require.True(t, s.Builtin, "合同状态必须是内置字段")
			require.Equal(t, s.Group, s.GroupLabel)
		}
	}
	require.Equal(t, 1, count[hrContractGroupLabor])
	require.Equal(t, 1, count[hrContractGroupService])
}

// 第 59 轮：旧胶囊名 → 新胶囊名，保留字段顺序与用户自定义标题。
func TestRound59RenameGroups(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "r1", TenantID: 1, Scope: hrScopeRoster, Name: "花名册", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Group: "在职管理", GroupLabel: "在职管理", Enabled: true},
			{Name: "姓名", Group: "在职管理", GroupLabel: "我的在职", Enabled: true},
			{Name: "离职日期", Group: "离职管理", GroupLabel: "离职管理", Enabled: true},
			{Name: "自定义列", Group: "自建分组", GroupLabel: "自建分组", Enabled: true},
		},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "s1", TenantID: 1, Scope: hrScopeSocial, Name: "社保管理", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "账单月份", Group: "异常记录", GroupLabel: "异常记录", Enabled: true},
		},
	}).Error)

	h := &HRHandler{db: db}
	require.NoError(t, h.hrRenameGroups59(ctx))

	var roster types.FleetCategory
	require.NoError(t, db.Where("id = ?", "r1").First(&roster).Error)
	require.Len(t, roster.Subs, 4, "顺序与字段数量不变")
	require.Equal(t, []string{"工号", "姓名", "离职日期", "自定义列"}, []string{
		roster.Subs[0].Name, roster.Subs[1].Name, roster.Subs[2].Name, roster.Subs[3].Name,
	})
	require.Equal(t, "在职人员", roster.Subs[0].Group)
	require.Equal(t, "在职人员", roster.Subs[0].GroupLabel)
	require.Equal(t, "在职人员", roster.Subs[1].Group)
	require.Equal(t, "我的在职", roster.Subs[1].GroupLabel, "用户自定义胶囊标题保留")
	require.Equal(t, "离职人员", roster.Subs[2].Group)
	require.Equal(t, "离职人员", roster.Subs[2].GroupLabel)
	require.Equal(t, "自建分组", roster.Subs[3].Group, "非旧名的分组不受影响")

	var social types.FleetCategory
	require.NoError(t, db.Where("id = ?", "s1").First(&social).Error)
	require.Equal(t, "补退记录", social.Subs[0].Group)
	require.Equal(t, "补退记录", social.Subs[0].GroupLabel)

	// 幂等
	require.NoError(t, h.hrRenameGroups59(ctx))
	require.NoError(t, db.Where("id = ?", "r1").First(&roster).Error)
	require.Equal(t, "在职人员", roster.Subs[0].Group)
}

// 第 59 轮：预警默认关闭 —— ListPFRecords 不追加虚拟待补录行，计数为 0。
func TestRound59AlertsDisabledByDefault(t *testing.T) {
	require.False(t, hrAlertsEnabled, "第 59 轮默认关闭异常统计")
	db := hrRound59DB(t)
	ctx := context.Background()

	// 在职且没有公积金账户 —— 打开开关时会产生「待补录」虚拟行。
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "在职甲", EmployeeID: "0001",
		IDNumber: "510232198001010011", Status: hrRosterStatusActive,
	}).Error)

	h := &HRHandler{db: db}
	require.Empty(t, h.hrFundPendingEmployees(ctx, 1))
	require.Empty(t, h.hrFundToSealEmployees(ctx, 1))
	require.Equal(t, 0, h.hrFundAlerts(ctx, 1).pending)
	require.Equal(t, 0, h.hrFundAlerts(ctx, 1).toSeal)
	require.Equal(t, 0, h.hrSocialAlerts(ctx, 1).pending)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(
			c.Request.Context(), types.TenantIDContextKey, uint64(1)))
		c.Next()
	})
	h.registerHRRoutes(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/hr/fund/records", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items       []map[string]any `json:"items"`
			Total       int              `json:"total"`
			Pending     int              `json:"pending_count"`
			ToSeal      int              `json:"to_seal_count"`
			StatusCount map[string]int64 `json:"status_counts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Empty(t, resp.Data.Items, "不得追加待补录 / 待封存虚拟行")
	require.Equal(t, 0, resp.Data.Pending)
	require.Equal(t, 0, resp.Data.ToSeal)
	require.Equal(t, int64(0), resp.Data.StatusCount["正常"])
}
