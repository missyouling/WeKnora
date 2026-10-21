package handler

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
)

// 回归（2026-10-21）：分类已删除，knowledge_extract_configs 里仍留着
// cert_type = 分类名 的配置行（真机 KB 0049ce9e 的「服务合同的副本1」）。
//
// 根因：提取配置按 **分类名字符串** 建键（kb × scope × cert_type），而
// DeleteFleetCategory 只软删 fleet_categories 一行，从不碰配置表。于是每删
// 一次分类就多一条孤儿配置；同名分类若日后被重建，用户会读到上一轮的旧规则。
//
// 契约：删除分类必须在**同一事务**内软删 (scope, cert_type=分类名) 的配置行，
// 且不得误伤其它 scope 的同名配置、不得误伤整体规则（cert_type 为空的行）。

func fleetCascadeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.FleetCategory{}, &types.KbExtractConfig{}, &types.Knowledge{}))
	return db
}

func fleetCascadeRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(1))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewFleetHandler(db)
	r.DELETE("/fleet/categories/:id", h.DeleteFleetCategory)
	return r
}

func TestDeleteFleetCategoryCascadesExtractConfig(t *testing.T) {
	db := fleetCascadeTestDB(t)
	cat := types.FleetCategory{ID: "cat-copy-1", TenantID: 0, Scope: "contract", Name: "服务合同的副本1"}
	require.NoError(t, db.Create(&cat).Error)

	orphan := types.KbExtractConfig{ID: "cfg-orphan", TenantID: 0, KnowledgeBaseID: "kb1", Scope: "contract", CertType: "服务合同的副本1", Enabled: true, Version: 1}
	otherScope := types.KbExtractConfig{ID: "cfg-other-scope", TenantID: 0, KnowledgeBaseID: "kb1", Scope: "invoice", CertType: "服务合同的副本1", Enabled: true, Version: 1}
	baseRule := types.KbExtractConfig{ID: "cfg-base", TenantID: 0, KnowledgeBaseID: "kb1", Scope: "contract", CertType: "", Enabled: true, Version: 1}
	otherCat := types.KbExtractConfig{ID: "cfg-other-cat", TenantID: 0, KnowledgeBaseID: "kb1", Scope: "contract", CertType: "租赁合同", Enabled: true, Version: 1}
	require.NoError(t, db.Create(&orphan).Error)
	require.NoError(t, db.Create(&otherScope).Error)
	require.NoError(t, db.Create(&baseRule).Error)
	require.NoError(t, db.Create(&otherCat).Error)

	r := fleetCascadeRouter(db)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/fleet/categories/cat-copy-1", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var got types.FleetCategory
	require.NoError(t, db.Unscoped().Where("id = ?", "cat-copy-1").First(&got).Error)
	assert.NotNil(t, got.DeletedAt, "分类必须被软删")

	var orphanAfter types.KbExtractConfig
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-orphan").First(&orphanAfter).Error)
	assert.NotNil(t, orphanAfter.DeletedAt, "同 scope 同名的提取配置必须被级联软删（否则留下孤儿配置）")

	var otherScopeAfter types.KbExtractConfig
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-other-scope").First(&otherScopeAfter).Error)
	assert.Nil(t, otherScopeAfter.DeletedAt, "不同 scope 的同名配置不得被误删")

	var baseAfter types.KbExtractConfig
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-base").First(&baseAfter).Error)
	assert.Nil(t, baseAfter.DeletedAt, "整体规则（cert_type 为空）不得被误删")

	var otherCatAfter types.KbExtractConfig
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-other-cat").First(&otherCatAfter).Error)
	assert.Nil(t, otherCatAfter.DeletedAt, "其它分类的配置不得被误删")
}

// 内置分类不可删除，也不得连带清理配置。
func TestDeleteFleetCategoryBuiltinKeepsConfig(t *testing.T) {
	db := fleetCascadeTestDB(t)
	cat := types.FleetCategory{ID: "cat-builtin", TenantID: 0, Scope: "contract", Name: "服务合同", BuiltinKey: "seed-contract"}
	require.NoError(t, db.Create(&cat).Error)
	cfg := types.KbExtractConfig{ID: "cfg-builtin", TenantID: 0, KnowledgeBaseID: "kb1", Scope: "contract", CertType: "服务合同", Enabled: true}
	require.NoError(t, db.Create(&cfg).Error)

	r := fleetCascadeRouter(db)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/fleet/categories/cat-builtin", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)

	var after types.KbExtractConfig
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-builtin").First(&after).Error)
	assert.Nil(t, after.DeletedAt, "删除被拒绝时不得清理配置")
}

// 车队档案分类（vehicle/driver/maintain）是按租户隔离的：同名分类在不同租户
// 下都存在是合法的，级联清理必须带 tenant_id，绝不能把别的租户的配置一起删掉。
func TestSoftDeleteExtractConfigsOfCategoryScopesByTenant(t *testing.T) {
	db := fleetCascadeTestDB(t)
	now := timeNowUTC()
	mine := types.KbExtractConfig{ID: "cfg-t1", TenantID: 1, KnowledgeBaseID: "kb1", Scope: "vehicle", CertType: "重型货车", Enabled: true, Version: 1}
	other := types.KbExtractConfig{ID: "cfg-t2", TenantID: 2, KnowledgeBaseID: "kb2", Scope: "vehicle", CertType: "重型货车", Enabled: true, Version: 1}
	require.NoError(t, db.Create(&mine).Error)
	require.NoError(t, db.Create(&other).Error)

	n, err := softDeleteExtractConfigsOfCategoryTx(context.Background(), db, "vehicle", "重型货车", 1, false, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "只清理本租户的那 1 行")

	var gotMine, gotOther types.KbExtractConfig
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-t1").First(&gotMine).Error)
	assert.NotNil(t, gotMine.DeletedAt)
	require.NoError(t, db.Unscoped().Where("id = ?", "cfg-t2").First(&gotOther).Error)
	assert.Nil(t, gotOther.DeletedAt, "其它租户的同名分类配置不得被误删")
}
