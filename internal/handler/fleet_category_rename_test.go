package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 回归（2026-10-20）：发票分类改名后存量票据全部掉进「其它票据」。
//
// 场景：把分类「水电发票」改名为「能耗发票」→ 原分类下 49 份票据被转移到
// 「其它票据」，改名后的分类显示 0 张。
//
// 根因：分类与票据之间是**分类名字符串**关联，共 4 处存量键：
//   custom_metadata.invoice_category / fleet_cert_type
//   custom_metadata.invoices[].invoice_type / invoices[].category
// 外加 knowledge_extract_configs.cert_type（按名建键）。
// 而 UpdateFleetCategory 只更新 fleet_categories.name 一列，改名完全不向
// 存量数据传播；前端「其它票据」又是排除式桶（不在启用分类即归桶），于是旧票
// 既不属于新分类、也没被任何启用分类命中 → 全部落进排除桶。
//
// 以下测试锁定修复契约：改名必须在同一事务内把上述名称引用整体改写；
// 目标配置已存在时整体回滚并返回 409（不静默覆盖、不留半截数据）。

func fleetRenameTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.FleetCategory{}, &types.Knowledge{},
		&types.KbExtractConfig{}, &types.KbExtractConfigVersion{},
	))
	return db
}

func fleetRenameRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// ErrorHandler 把 c.Error() 转成 JSON 错误响应（生产同款中间件），
	// 否则 c.Error 只入 ctx.Errors，响应停在 200 空体。
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		// fleetTenantID 走 c.Request.Context()，tenant 必须写进请求上下文而非 gin Keys
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(1))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewFleetHandler(db)
	r.PUT("/fleet/categories/:id", h.UpdateFleetCategory)
	return r
}

// invoiceMetaForRename 造一份带分类名的发票文件 metadata（4 处存量键全部写上）。
// invoice_no 用固定值（不掺分类名），保证「整串不含旧分类名」的断言有意义。
func invoiceMetaForRename(cat string, omitFileLevelKeys bool) []byte {
	m := map[string]interface{}{
		"kind": "invoice", "extract_status": "success",
		"invoices": []interface{}{
			map[string]interface{}{
				"invoice_no": "INV-001", "invoice_type": cat, "category": cat,
				"total_amount": 1234.5, "tax_rate": 0.09, "page": 1,
			},
		},
	}
	if !omitFileLevelKeys {
		m["invoice_category"] = cat
		m["fleet_cert_type"] = cat
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

// renameMetaKeys 取出 metadata 里与分类名相关的键值，便于逐键断言。
type renameMetaKeys struct {
	InvoiceCategory string `json:"invoice_category"`
	FleetCertType   string `json:"fleet_cert_type"`
	Invoices        []struct {
		InvoiceType string   `json:"invoice_type"`
		Category    string   `json:"category"`
		InvoiceNo   string   `json:"invoice_no"`
		TotalAmount *float64 `json:"total_amount"`
	} `json:"invoices"`
}

func readRenameMeta(t *testing.T, db *gorm.DB, id string) renameMetaKeys {
	t.Helper()
	var k types.Knowledge
	require.NoError(t, db.Where("id = ?", id).First(&k).Error)
	var out renameMetaKeys
	require.NoError(t, json.Unmarshal(k.CustomMetadata, &out))
	return out
}

func putFleetCategoryRename(t *testing.T, r *gin.Engine, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/fleet/categories/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestBusinessRenameInvoiceCategoryCarriesInvoices 是本次 bug 的主回归：
// 改名后 4 处存量键 + 提取配置 cert_type 必须整体切换到新名，
// 且不改动其它分类的数据、不破坏票据自身字段。
func TestBusinessRenameInvoiceCategoryCarriesInvoices(t *testing.T) {
	db := fleetRenameTestDB(t)
	const oldName, newName = "水电发票", "能耗发票"

	cat := types.FleetCategory{
		ID: "cat-hydro", TenantID: 0, Scope: "invoice",
		Name: oldName, Enabled: true, SortOrder: 1, Subs: []types.FleetCategorySub{},
	}
	require.NoError(t, db.Create(&cat).Error)

	// 3 份旧分类文件（其中 k2 缺 fleet_cert_type，只带权威键 invoice_category）
	k1 := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", Title: "水电1", FileName: "a.pdf", CustomMetadata: types.JSON(invoiceMetaForRename(oldName, false))}
	k2 := types.Knowledge{ID: "k2", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", Title: "水电2", FileName: "b.pdf", CustomMetadata: types.JSON(invoiceMetaForRename(oldName, true))}
	// k3 属于别的分类，绝不能被动
	k3 := types.Knowledge{ID: "k3", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", Title: "通行费", FileName: "c.pdf", CustomMetadata: types.JSON(invoiceMetaForRename("通行费发票", false))}
	for _, k := range []*types.Knowledge{&k1, &k2, &k3} {
		require.NoError(t, db.Create(k).Error)
	}

	// 提取配置：按 kb × scope × cert_type 建键，改名必须跟随
	cfg := types.KbExtractConfig{ID: "cfg1", TenantID: 1, KnowledgeBaseID: "kb1", Scope: "invoice", CertType: oldName, Fields: []types.ExtractFieldConfig{}, Version: 14, Enabled: true, PromptTemplate: "tmpl-hydro"}
	require.NoError(t, db.Create(&cfg).Error)
	ver := types.KbExtractConfigVersion{ID: "ver1", ConfigID: "cfg1", Version: 14, PromptTemplate: "tmpl-hydro"}
	require.NoError(t, db.Create(&ver).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-hydro", `{"name":"`+newName+`","enabled":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 1) 分类名已改
	var gotCat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-hydro").First(&gotCat).Error)
	require.Equal(t, newName, gotCat.Name)

	// 2) 存量 metadata 的票内两键整体切换。文件级两键只在原本就存在的行上断言：
	//    k1 全量带，k2 刻意只带票内键（缺文件级 invoice_category/fleet_cert_type 的
	//    存量形态）—— 传播只应改写已有键，不得凭空造键。
	for _, tc := range []struct {
		id              string
		hasFileLevelKey bool
	}{
		{"k1", true},
		{"k2", false},
	} {
		after := readRenameMeta(t, db, tc.id)
		if tc.hasFileLevelKey {
			require.Equal(t, newName, after.InvoiceCategory, "k %s invoice_category", tc.id)
			require.Equal(t, newName, after.FleetCertType, "k %s fleet_cert_type", tc.id)
		} else {
			require.Empty(t, after.InvoiceCategory, "k %s 不应被凭空造键", tc.id)
			require.Empty(t, after.FleetCertType, "k %s 不应被凭空造键", tc.id)
		}
		require.Len(t, after.Invoices, 1, "k %s invoices", tc.id)
		require.Equal(t, newName, after.Invoices[0].InvoiceType, "k %s invoice_type", tc.id)
		require.Equal(t, newName, after.Invoices[0].Category, "k %s category", tc.id)
		// 票据自身字段不受改名影响
		require.Equal(t, "INV-001", after.Invoices[0].InvoiceNo, "k %s invoice_no 不得被改写", tc.id)
		require.NotNil(t, after.Invoices[0].TotalAmount, "k %s total_amount 不得丢", tc.id)
		require.Equal(t, 1234.5, *after.Invoices[0].TotalAmount, "k %s total_amount 精度", tc.id)
		require.NotContains(t, string(readRawMetadata(t, db, tc.id)), oldName, "k %s 不得残留旧分类名", tc.id)
	}

	// 3) 别的分类分毫未动
	k3After := readRenameMeta(t, db, "k3")
	require.Equal(t, "通行费发票", k3After.InvoiceCategory)
	require.Equal(t, "通行费发票", k3After.FleetCertType)
	require.Equal(t, "通行费发票", k3After.Invoices[0].InvoiceType)

	// 4) 提取配置跟随改名，版本快照仍挂在原配置下
	var cfgAfter types.KbExtractConfig
	require.NoError(t, db.Where("id = ?", "cfg1").First(&cfgAfter).Error)
	require.Equal(t, newName, cfgAfter.CertType)
	require.Equal(t, 14, cfgAfter.Version, "改名不得重置配置版本")
	require.Equal(t, "tmpl-hydro", cfgAfter.PromptTemplate, "改名不得丢规则底稿")
	var verAfter types.KbExtractConfigVersion
	require.NoError(t, db.Where("config_id = ?", "cfg1").First(&verAfter).Error)
	require.Equal(t, 14, verAfter.Version)

	// 5) 旧名下不应再有任何引用
	var leftover int64
	require.NoError(t, db.Model(&types.FleetCategory{}).Where("name = ?", oldName).Count(&leftover).Error)
	require.Zero(t, leftover)
}

func readRawMetadata(t *testing.T, db *gorm.DB, id string) types.JSON {
	t.Helper()
	var k types.Knowledge
	require.NoError(t, db.Where("id = ?", id).First(&k).Error)
	return k.CustomMetadata
}

// TestBusinessRenameInvoiceCategoryRejectsExtractConfigConflict 锁定冲突保护：
// 同一知识库下已存在目标名配置时拒绝改名并整体回滚（含分类名与存量数据），
// 绝不清覆盖用户的另一份配置、也绝不留半截改名状态。
func TestBusinessRenameInvoiceCategoryRejectsExtractConfigConflict(t *testing.T) {
	db := fleetRenameTestDB(t)
	const oldName, newName = "水电发票", "能耗发票"

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-x", TenantID: 0, Scope: "invoice", Name: oldName, Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	k := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(invoiceMetaForRename(oldName, false))}
	require.NoError(t, db.Create(&k).Error)
	// 同一 kb 下新旧两份配置并存 → 改名会产生歧义，必须拒绝
	require.NoError(t, db.Create(&types.KbExtractConfig{ID: "cfg-old", TenantID: 1, KnowledgeBaseID: "kb1", Scope: "invoice", CertType: oldName, Fields: []types.ExtractFieldConfig{}, Enabled: true}).Error)
	require.NoError(t, db.Create(&types.KbExtractConfig{ID: "cfg-new", TenantID: 1, KnowledgeBaseID: "kb1", Scope: "invoice", CertType: newName, Fields: []types.ExtractFieldConfig{}, Enabled: true}).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-x", `{"name":"`+newName+`"}`)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	// 分类名保持旧值
	var gotCat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-x").First(&gotCat).Error)
	require.Equal(t, oldName, gotCat.Name)
	// 存量 metadata 未被局部改写（事务整体回滚）
	after := readRenameMeta(t, db, "k1")
	require.Equal(t, oldName, after.InvoiceCategory)
	require.Equal(t, oldName, after.Invoices[0].Category)
	// 两份配置都保持原状
	var cnt int64
	require.NoError(t, db.Model(&types.KbExtractConfig{}).Count(&cnt).Error)
	require.Equal(t, int64(2), cnt)
}

// TestBusinessRenameInvoiceCategoryNameUnchangedNoop 锁定幂等：
// 名字没变时不触发任何传播（避免无谓的全表扫描与 updated_at 抖动）。
func TestBusinessRenameInvoiceCategoryNameUnchangedNoop(t *testing.T) {
	db := fleetRenameTestDB(t)
	const name = "水电发票"
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-keep", TenantID: 0, Scope: "invoice", Name: name, Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	k := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(invoiceMetaForRename(name, false))}
	require.NoError(t, db.Create(&k).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-keep", `{"name":"`+name+`","enabled":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	after := readRenameMeta(t, db, "k1")
	require.Equal(t, name, after.InvoiceCategory)
	require.Equal(t, name, after.FleetCertType)
	require.Equal(t, name, after.Invoices[0].InvoiceType)
	require.Equal(t, name, after.Invoices[0].Category)
}

// TestBusinessRenameCategorySubstringSafety 锁定替换粒度：
// 只允许整值相等才替换。把「水」改名为「水源」时，值「水费发票」的票
// 绝不能被误改成「水源费发票」。
func TestBusinessRenameCategorySubstringSafety(t *testing.T) {
	db := fleetRenameTestDB(t)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-short", TenantID: 0, Scope: "invoice", Name: "水", Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	// 一条值恰为「水」，另一条值「水费发票」（仅子串命中）
	same := types.Knowledge{ID: "k-same", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(invoiceMetaForRename("水", false))}
	longer := types.Knowledge{ID: "k-long", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(invoiceMetaForRename("水费发票", false))}
	require.NoError(t, db.Create(&same).Error)
	require.NoError(t, db.Create(&longer).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-short", `{"name":"水源"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	sameAfter := readRenameMeta(t, db, "k-same")
	require.Equal(t, "水源", sameAfter.InvoiceCategory)
	require.Equal(t, "水源", sameAfter.Invoices[0].Category)

	longAfter := readRenameMeta(t, db, "k-long")
	require.Equal(t, "水费发票", longAfter.InvoiceCategory)
	require.Equal(t, "水费发票", longAfter.Invoices[0].Category)
}
