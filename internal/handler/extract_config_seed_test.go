package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 回归（2026-10-21）：四模块（合同/制度/通知公告）提取规则底稿启动回填。
//
// 契约：
//  1. 存量行只补 desc/rule/regex/type/prompt_template 的空值，用户改过的文案一字不动；
//  2. 目录里有但配置里没有的字段不硬塞（用户可能主动删过，复活会打乱其配置）；
//  3. 缺失配置的 知识库×scope 建一份整体规则底稿（cert_type 为空）；
//  4. 重复启动完全幂等：不新增行、不产生写库。
func businessSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.KnowledgeBase{}, &types.KbExtractConfig{}))
	return db
}

func TestBusinessExtractConfigSeedBackfill(t *testing.T) {
	db := businessSeedTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-1", TenantID: 7, Name: "日常事务库"}).Error)

	// 存量合同配置：两条字段（其中一条 desc 被用户改过文案），无 prompt_template。
	legacy := types.KbExtractConfig{
		ID: "cfg-contract", TenantID: 7, KnowledgeBaseID: "kb-1",
		Scope: "contract", CertType: "", Version: 3, Enabled: true,
		Fields: []types.ExtractFieldConfig{
			{Name: "contract_no", Label: "合同编号", Desc: "用户自定义：只取合同右上角编号", Rule: ""},
			{Name: "amount", Label: "金额"},
		},
	}
	require.NoError(t, db.Create(&legacy).Error)

	h := &BusinessExtractHandler{db: db}
	h.SeedBusinessExtractConfigBackfill(ctx)

	var patched types.KbExtractConfig
	require.NoError(t, db.Where("id = ?", "cfg-contract").First(&patched).Error)
	require.Len(t, patched.Fields, 2, "目录里有但配置里没有的字段不得硬塞")
	assert.Equal(t, "用户自定义：只取合同右上角编号", patched.Fields[0].Desc, "用户改过的 desc 不得覆盖")
	assert.NotEmpty(t, patched.Fields[0].Rule, "空 rule 必须按目录补齐")
	assert.NotEmpty(t, patched.Fields[1].Desc, "空 desc 必须按目录补齐")
	assert.NotEmpty(t, patched.PromptTemplate, "空高级模板必须补默认模板")
	assert.EqualValues(t, 3, patched.Version, "回填不推进版本号（非用户保存）")

	// 该知识库的另外两个 scope 各建一份整体规则底稿
	for _, scope := range []string{"regulation", "award_punish"} {
		var row types.KbExtractConfig
		require.NoError(t, db.Where("knowledge_base_id = ? AND scope = ? AND cert_type = ?", "kb-1", scope, "").
			First(&row).Error, "缺少 %s 整体规则底稿", scope)
		assert.NotEmpty(t, row.Fields, "%s 底稿字段不得为空", scope)
		assert.NotEmpty(t, row.PromptTemplate, "%s 底稿高级模板不得为空", scope)
	}
	var contractCount int64
	require.NoError(t, db.Model(&types.KbExtractConfig{}).
		Where("knowledge_base_id = ? AND scope = ?", "kb-1", "contract").Count(&contractCount).Error)
	assert.EqualValues(t, 1, contractCount, "已存在配置时不得再建整体规则（避免两套并存）")

	// 幂等：第二次回填不新增行、不改任何行
	var before []types.KbExtractConfig
	require.NoError(t, db.Order("id").Find(&before).Error)
	beforeSnap, err := json.Marshal(before)
	require.NoError(t, err)
	h.SeedBusinessExtractConfigBackfill(ctx)
	var after []types.KbExtractConfig
	require.NoError(t, db.Order("id").Find(&after).Error)
	afterSnap, err := json.Marshal(after)
	require.NoError(t, err)
	assert.JSONEq(t, string(beforeSnap), string(afterSnap), "重复启动必须完全幂等")
}

// 已填满的配置不应产生任何写库（updated_at 不变）。
func TestBusinessExtractConfigSeedNoopWhenComplete(t *testing.T) {
	db := businessSeedTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-2", TenantID: 1, Name: "库"}).Error)
	h := &BusinessExtractHandler{db: db}
	h.SeedBusinessExtractConfigBackfill(ctx)

	var first types.KbExtractConfig
	require.NoError(t, db.Where("knowledge_base_id = ? AND scope = ?", "kb-2", "contract").First(&first).Error)
	stamp := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, db.Model(&types.KbExtractConfig{}).Where("id = ?", first.ID).
		UpdateColumn("updated_at", stamp).Error)

	h.SeedBusinessExtractConfigBackfill(ctx)

	var again types.KbExtractConfig
	require.NoError(t, db.Where("id = ?", first.ID).First(&again).Error)
	assert.True(t, again.UpdatedAt.UTC().Equal(stamp), "已完整配置不得被回填改写（updated_at 应保持 %s，实际 %s）",
		stamp, again.UpdatedAt.UTC())
}
