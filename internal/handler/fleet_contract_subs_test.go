package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 回归（2026-10-21）：合同抽屉字段顺序 = subs 顺序，subs 顺序 = 契约清单顺序。
//
// 历史 subs 里「税率」夹在金额与提取状态之间、扩展字段散在末尾、还混着中文名旧键
// （甲方经办人/摘要），抽屉就会渲染成「金额 → 税率 → 提取状态 → … → 甲方经办人」。
// 以下测试锁定：归一后严格按 11 标准 → 15 扩展 排列，且保留用户开关。
//
// 2026-10-21 二次调整：`fileName` 是**列表系统列**（文件属性，不是提取字段），
// 已从 seed 与 subs 中剥离——它会渗进编辑抽屉（末尾冒出「文件名称」输入框）与
// 提取规则表，成为用户在字段定义里找不到来历的幽灵字段。列表侧由前端
// `appendListSystemColumns` 显式追加「文件名」列，不依赖 subs。
func TestMergeContractSeedSubsOrder(t *testing.T) {
	legacy := []types.FleetCategorySub{
		{Name: "contract_no", Enabled: true, IsDefault: true, DataType: "text", Width: 160},
		{Name: "contract_name", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "partyAName", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "partyBName", Enabled: false, IsDefault: true, DataType: "text"}, // 用户关掉过乙方
		{Name: "tax_rate", Enabled: true, IsDefault: false, DataType: "number"},
		{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "fulfillStatus", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "甲方经办人", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "summary", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "file_name", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
	}

	out := mergeContractSeedSubs(legacy)
	names := make([]string, 0, len(out))
	for _, s := range out {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{
		"contract_no", "contract_name", "contract_type", "party_a", "party_b",
		"sign_date", "expire_date", "amount", "extract_status", "performance_status", "tags",
		"party_a_contact", "party_a_tax_no", "party_b_contact", "party_b_tax_no",
		"party_a_address", "party_a_phone", "party_a_bank", "party_a_account",
		"party_b_address", "party_b_phone", "party_b_bank", "party_b_account",
		"payment_method", "sign_location", "summary",
	}, names, "subs 必须收敛到契约清单顺序，且不含 fileName 系统列")

	// 用户配置不被覆盖；旧键值被搬到新键上
	require.Equal(t, 160, out[0].Width, "用户设置的列宽必须保留")
	var partyB *types.FleetCategorySub
	for i := range out {
		if out[i].Name == "party_b" {
			partyB = &out[i]
		}
	}
	require.NotNil(t, partyB)
	assert.False(t, partyB.Enabled, "用户关掉的乙方字段不得被打开")

	// 历史多余字段（税率/中文旧键/系统列）不进展示字段集
	for _, s := range out {
		assert.NotContains(t, []string{"tax_rate", "甲方经办人", "file_name", "fileName"}, s.Name,
			"历史字段 %s 不应进入契约 subs", s.Name)
	}
}

// 通知公告与奖惩共用同一份字段目录：subs 顺序必须等于统一目录顺序，
// 目录外历史键（dept/position/remark）与系统列 fileName 不再进入展示集。
func TestMergeNoticeSeedSubsOrder(t *testing.T) {
	legacy := []types.FleetCategorySub{
		{Name: "apNo"}, {Name: "apTitle"}, {Name: "apType"},
		{Name: "signDate"}, {Name: "signer"}, {Name: "issueDept"},
		{Name: "summary"}, {Name: "person"}, {Name: "dept"},
		{Name: "position"}, {Name: "measure"}, {Name: "basis"},
		{Name: "effectiveDate"}, {Name: "remark"}, {Name: "extract_status"},
		{Name: "tags"}, {Name: "fileName"}, {Name: "scope"},
		{Name: "contact"}, {Name: "contactPhone"}, {Name: "urgency"},
		{Name: "valid_until"}, {Name: "attachments"},
	}
	out := mergeNoticeSeedSubs(legacy)
	names := make([]string, 0, len(out))
	for _, s := range out {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{
		"apNo", "apTitle", "apType", "issueDept", "signDate", "effectiveDate", "scope", "extractStatus", "tags",
		"signer", "contact", "contactPhone", "urgency", "validUntil", "attachments", "person", "measure", "basis", "summary",
	}, names, "公告 subs 必须收敛到统一目录顺序，且不含 fileName 系统列")
	for _, s := range out {
		assert.NotContains(t, []string{"dept", "position", "remark", "fileName"}, s.Name, "历史键 %s 不应进入目录", s.Name)
	}
}
