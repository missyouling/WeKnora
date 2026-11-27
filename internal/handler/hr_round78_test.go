// 第 78 轮回归：
//  1. 维保记录收敛：一般维修 / 二级维护重建字段，合格证分类停用、存量记录映射零丢失
//  2. 随单附件袋：BYTEA 上传 / 列表不含二进制 / 无后缀预览 / 删除，且**零触发解析**
//  3. 行驶证补「年检日期」字段与提取规则（幂等、只补空规则）
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// stripGoComments 去掉 Go 源码里的行注释与块注释（保留字符串字面量里的内容）。
func stripGoComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	runes := []rune(src)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == '"' {
			// 跳过字符串字面量（含转义），原样保留
			b.WriteRune(c)
			for i++; i < len(runes); i++ {
				b.WriteRune(runes[i])
				if runes[i] == '\\' && i+1 < len(runes) {
					i++
					b.WriteRune(runes[i])
					continue
				}
				if runes[i] == '"' {
					break
				}
			}
			continue
		}
		if c == '/' && i+1 < len(runes) {
			if runes[i+1] == '/' {
				for i < len(runes) && runes[i] != '\n' {
					i++
				}
				b.WriteRune('\n')
				continue
			}
			if runes[i+1] == '*' {
				i += 2
				for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
					i++
				}
				i++
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

// readHandlerSource 读取同目录下的 handler 源码（用于源码层铁律扫描）。
func readHandlerSource(name string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(wd, name))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func hrRound78DB(t *testing.T) *gorm.DB {
	t.Helper()
	db := hrRound70DB(t)
	require.NoError(t, db.AutoMigrate(&types.FleetRecord{}, &types.KbExtractConfig{}, &FleetRecordAttachment{}, &types.KnowledgeBase{}))
	return db
}

// ---------- ① 维保分类收敛 ----------
func TestRound78ReconfigMaintenance(t *testing.T) {
	db := hrRound78DB(t)
	ctx := context.Background()

	// 旧版 3 个分类：一般维修 / 合格证 / 二级维护（二级维护此刻还不存在）
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-repair", TenantID: 1, Scope: "maintain", Name: "一般维修", Enabled: true, SortOrder: 1,
		Subs: []types.FleetCategorySub{
			// 用户手工调过宽度与文案：升级后必须保留
			{Name: "工时费", Label: "工时费(元)", Enabled: true, IsDefault: true, DataType: "text", Width: 120},
			{Name: "用户自建字段", Enabled: true, DataType: "text"},
		},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-cert", TenantID: 1, Scope: "maintain", Name: "合格证", Enabled: true, SortOrder: 2,
		Subs: []types.FleetCategorySub{{Name: "拖修方", Enabled: true, DataType: "text"}},
	}).Error)
	// 存量合格证记录（旧字段名）
	require.NoError(t, db.Create(&types.FleetRecord{
		ID: "rec-cert", TenantID: 1, RecordType: "maintain-archive", SubCategory: "合格证", DocType: "合格证",
		Data: map[string]any{
			"拖修方": "重庆星达铜业有限公司", "车牌号码": "渝C81022",
			"底盘（车身）号": "LVSHCAMB7AE123456", "出厂里程表示值": float64(120000),
			"质检员": "王质检", "接车人": "李接车", "合格证编号": "4926591",
		},
	}).Error)

	h := &HRHandler{db: db}
	repairFields, maintFields, migrated, _ := h.hrReconfigMaintenance78(ctx, 1)

	require.GreaterOrEqual(t, repairFields, 23, "一般维修至少 23 个标准字段")
	require.GreaterOrEqual(t, maintFields, 22, "二级维护至少 22 个标准字段")
	require.Equal(t, 1, migrated, "存量合格证记录必须迁移 1 条")

	// 一般维修：新字段齐全 + 用户调过的宽度/文案/自建字段都还在
	var repair types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-repair").First(&repair).Error)
	names := map[string]types.FleetCategorySub{}
	for _, s := range repair.Subs {
		names[s.Name] = s
	}
	for _, want := range []string{"结算单号", "车牌号", "结算日期", "挂账单位", "维修项目摘要",
		"工时费", "材料费", "管理费", "其它费", "实际应收", "承修单位"} {
		require.Contains(t, names, want, "一般维修缺字段 %s", want)
	}
	require.Equal(t, 120, names["工时费"].Width, "用户调过的列宽必须保留")
	require.Equal(t, "工时费(元)", names["工时费"].Label, "用户改过的文案必须保留")
	require.Contains(t, names, "用户自建字段", "用户自建字段不得被删")
	require.Equal(t, "amount", names["实际应收"].DataType)
	// 分组必须落在 3 个胶囊里
	groups := map[string]int{}
	for _, s := range repair.Subs {
		if s.Group != "" {
			groups[s.Group]++
		}
	}
	require.Equal(t, 10, groups["基本信息"])
	require.Equal(t, 7, groups["费用汇总"])
	require.Equal(t, 6, groups["承修单位"])

	// 二级维护：新字段齐全
	var maint types.FleetCategory
	require.NoError(t, db.Where("tenant_id = ? AND scope = ? AND name = ?", 1, "maintain", "二级维护").
		First(&maint).Error)
	mnames := map[string]bool{}
	for _, s := range maint.Subs {
		mnames[s.Name] = true
	}
	for _, want := range []string{"合格证编号", "托修方", "维修类别", "出厂日期", "车牌号",
		"出厂里程", "质保里程", "质保天数", "承修方", "质量检验员"} {
		require.True(t, mnames[want], "二级维护缺字段 %s", want)
	}

	// 合格证分类必须停用（记录已迁走）
	var cert types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-cert").First(&cert).Error)
	require.False(t, cert.Enabled, "合格证分类必须停用")

	// 存量记录：sub_category/doc_type 迁到二级维护，旧键保留 + 新键补齐
	var rec types.FleetRecord
	require.NoError(t, db.Where("id = ?", "rec-cert").First(&rec).Error)
	require.Equal(t, "二级维护", rec.SubCategory)
	require.Equal(t, "二级维护", rec.DocType)
	require.Equal(t, "重庆星达铜业有限公司", rec.Data["拖修方"], "旧键必须保留（零丢失）")
	require.Equal(t, "重庆星达铜业有限公司", rec.Data["托修方"], "新键必须补齐")
	require.Equal(t, "渝C81022", rec.Data["车牌号"])
	require.Equal(t, "LVSHCAMB7AE123456", rec.Data["车架号"])
	require.Equal(t, "王质检", rec.Data["质量检验员"])
	require.Equal(t, "李接车", rec.Data["托修方接车人"])
	require.Equal(t, "4926591", rec.Data["合格证编号"])

	// 幂等：再跑一次不再迁移、不再重复加字段
	_, _, migrated2, _ := h.hrReconfigMaintenance78(ctx, 1)
	require.Equal(t, 0, migrated2, "第二次迁移必须为 0（幂等）")
	var repair2 types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-repair").First(&repair2).Error)
	require.Equal(t, len(repair.Subs), len(repair2.Subs), "幂等：字段数不得增长")
}

// ---------- ② 行驶证年检日期 ----------
func TestRound78AddDrivingLicenseInspection(t *testing.T) {
	db := hrRound78DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-license", TenantID: 1, Scope: "vehicle", Name: "行驶证", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "车牌号", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "强制报废日期", Enabled: true, IsDefault: true, DataType: "date"},
			{Name: "证件状态", Enabled: true, IsDefault: true, DataType: "text"},
		},
	}).Error)
	// 车队知识库（迁移按 kb 名含「车队」播种提取规则）
	require.NoError(t, db.Create(&types.KnowledgeBase{
		ID: "kb1", TenantID: 1, Name: "日常事务-车队",
	}).Error)
	// 已有行驶证提取配置（含车牌号规则）
	require.NoError(t, db.Create(&types.KbExtractConfig{
		ID: "cfg-license", TenantID: 1, KnowledgeBaseID: "kb1", Scope: "vehicle", CertType: "行驶证",
		Enabled: true,
		Fields: []types.ExtractFieldConfig{
			{Name: "车牌号", Enabled: true, Regex: `车牌号[:：]\s*(\S+)`},
		},
	}).Error)

	h := &HRHandler{db: db}
	added, _ := h.hrAddDrivingLicenseInspection78(ctx, 1)
	require.Equal(t, 1, added)

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-license").First(&cat).Error)
	at, found := -1, false
	for i, s := range cat.Subs {
		if s.Name == "年检日期" {
			found = true
			at = i
			require.True(t, s.Enabled)
			require.True(t, s.IsDefault, "年检日期必须在默认表头里")
			require.Equal(t, "date", s.DataType)
		}
	}
	require.True(t, found, "行驶证必须补年检日期")
	// 位置紧跟「强制报废日期」（语义相邻）
	require.Equal(t, "强制报废日期", cat.Subs[at-1].Name, "年检日期应紧跟强制报废日期")

	// 提取规则：并入年检日期规则，原有车牌号规则不得被覆盖
	var cfg types.KbExtractConfig
	require.NoError(t, db.Where("id = ?", "cfg-license").First(&cfg).Error)
	byName := map[string]types.ExtractFieldConfig{}
	for _, f := range cfg.Fields {
		byName[f.Name] = f
	}
	require.Contains(t, byName, "年检日期")
	require.NotEmpty(t, byName["年检日期"].Regex)
	require.Contains(t, byName["年检日期"].Regex, "检验有效期至")
	require.Equal(t, `车牌号[:：]\s*(\S+)`, byName["车牌号"].Regex, "用户已有规则不得被覆盖")

	// 幂等：再跑不重复加字段
	added2, _ := h.hrAddDrivingLicenseInspection78(ctx, 1)
	require.Equal(t, 0, added2)
	var cat2 types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-license").First(&cat2).Error)
	n := 0
	for _, s := range cat2.Subs {
		if s.Name == "年检日期" {
			n++
		}
	}
	require.Equal(t, 1, n, "年检日期不得重复")
}

// ---------- ③ 随单附件袋 ----------
func TestRound78AttachmentLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := hrRound78DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetRecord{
		ID: "rec-maint", TenantID: 1, RecordType: "maintain-archive", SubCategory: "二级维护",
		Data: map[string]any{"合格证编号": "4926591"},
	}).Error)

	h := &FleetHandler{db: db}
	h.ensureFleetAttachmentSchema(ctx)

	png := []byte("\x89PNG\r\n\x1a\nfakepngbytes")
	// 上传（multipart，多文件）
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("tag", "竣工检测单")
	for i, name := range []string{"check1.png", "check2.png"} {
		fw, err := w.CreateFormFile("files", name)
		require.NoError(t, err)
		_, _ = fw.Write(append(png, byte(i)))
	}
	require.NoError(t, w.Close())

	r := httptest.NewRequest(http.MethodPost, "/api/v1/fleet/records/rec-maint/attachments", body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r = r.WithContext(context.WithValue(r.Context(), types.TenantIDContextKey, uint64(1)))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = r
	c.Params = gin.Params{{Key: "id", Value: "rec-maint"}}
	h.UploadFleetRecordAttachments(c)
	require.Equal(t, http.StatusOK, c.Writer.Status())

	// 落库 2 条，且 BYTEA 真的有内容
	var rows []FleetRecordAttachment
	require.NoError(t, db.Where("record_id = ?", "rec-maint").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, "竣工检测单", rows[0].Tag)
	require.Equal(t, "image/png", rows[0].MimeType)
	require.NotEmpty(t, rows[0].Content)
	require.Equal(t, int64(len(png)+1), rows[0].FileSize)

	// 列表接口：JSON 里绝不能出现 BYTEA
	rec := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec)
	r2req := httptest.NewRequest(http.MethodGet, "/api/v1/fleet/records/rec-maint/attachments", nil)
	c2.Request = r2req.WithContext(context.WithValue(r2req.Context(), types.TenantIDContextKey, uint64(1)))
	c2.Params = gin.Params{{Key: "id", Value: "rec-maint"}}
	h.ListFleetRecordAttachments(c2)
	require.Equal(t, http.StatusOK, c2.Writer.Status())
	payload := rec.Body.String()
	require.NotContains(t, payload, "fakepngbytes", "列表接口严禁返回 BYTEA")
	require.Contains(t, payload, "check1.png")
	var listed struct {
		Data struct {
			Items []FleetRecordAttachment `json:"items"`
			Tags  []string                `json:"tags"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Data.Items, 2)
	require.Contains(t, listed.Data.Tags, "过程照片")

	// 无后缀预览：Content-Type 与内容都要对
	rec2 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(rec2)
	r3req := httptest.NewRequest(http.MethodGet, "/api/v1/fleet/attachments/view?id="+rows[0].ID, nil)
	c3.Request = r3req.WithContext(context.WithValue(r3req.Context(), types.TenantIDContextKey, uint64(1)))
	h.ViewFleetAttachment(c3)
	require.Equal(t, http.StatusOK, c3.Writer.Status())
	require.Equal(t, "image/png", rec2.Header().Get("Content-Type"))
	require.Equal(t, "inline", rec2.Header().Get("Content-Disposition"))
	require.True(t, bytes.HasPrefix(rec2.Body.Bytes(), png))

	// 计数（列表「附件」列）
	counts := h.CountFleetRecordAttachments(ctx, 1, []string{"rec-maint"})
	require.Equal(t, 2, counts["rec-maint"])

	// 删除一个 → 计数变 1
	rec3 := httptest.NewRecorder()
	c4, _ := gin.CreateTestContext(rec3)
	r4req := httptest.NewRequest(http.MethodDelete, "/api/v1/fleet/attachments/"+rows[0].ID, nil)
	c4.Request = r4req.WithContext(context.WithValue(r4req.Context(), types.TenantIDContextKey, uint64(1)))
	c4.Params = gin.Params{{Key: "attID", Value: rows[0].ID}}
	h.DeleteFleetAttachment(c4)
	require.Equal(t, http.StatusOK, c4.Writer.Status())
	counts2 := h.CountFleetRecordAttachments(ctx, 1, []string{"rec-maint"})
	require.Equal(t, 1, counts2["rec-maint"])

	// 越权：别的租户读不到（租户隔离）
	rec4 := httptest.NewRecorder()
	c5, _ := gin.CreateTestContext(rec4)
	r5req := httptest.NewRequest(http.MethodGet, "/api/v1/fleet/attachments/view?id="+rows[1].ID, nil)
	c5.Request = r5req.WithContext(context.WithValue(r5req.Context(), types.TenantIDContextKey, uint64(2)))
	h.ViewFleetAttachment(c5)
	// gin 的 c.Error 只登记错误、不写状态码（由全局错误中间件转 JSON），
	// 因此这里判 c.Errors 而不是 Writer.Status()。
	require.NotEmpty(t, c5.Errors, "跨租户预览必须被拒")
	require.Empty(t, rec4.Body.String(), "跨租户不得返回任何内容")
}

// 附件上传**零解析触发**：源码层扫描（比运行期 mock 更彻底 —— 只要不引用就不会触发）。
func TestRound78AttachmentNeverTriggersExtract(t *testing.T) {
	src, err := readHandlerSource("fleet_attachments.go")
	require.NoError(t, err)
	// 剥掉注释再扫：说明文字里会提到「严禁触发 chatModel」这类字样，
	// 留在代码里的真实调用才是要拦的对象。
	code := stripGoComments(src)
	for _, banned := range []string{
		"ExtractFleetDocument", "RunExtractWithRetry", "chatModel",
		"ExtractUtilityBill", "ValidateGenericExtraction", "KbExtractConfig",
	} {
		require.NotContains(t, code, banned,
			"附件袋铁律：上传/预览链路严禁出现 %s（只存不解）", banned)
	}
}

// 不支持的 MIME 必须被拒（避免把任意文件塞进 PG）。
func TestRound78AttachmentRejectsBadMime(t *testing.T) {
	require.Equal(t, "", fleetAttachmentMimeOf("application/zip", "a.zip"))
	require.Equal(t, "", fleetAttachmentMimeOf("text/plain", "a.txt"))
	require.Equal(t, "image/jpeg", fleetAttachmentMimeOf("image/jpg", "a.jpg"))
	require.Equal(t, "application/pdf", fleetAttachmentMimeOf("application/octet-stream", "a.pdf"))
	require.Equal(t, "image/webp", fleetAttachmentMimeOf("image/webp", "a.webp"))
}

// 维保分类的提取规则必须覆盖三组明细数组（否则模型不会输出子表）。
func TestRound78RepairExtractCoversLineItems(t *testing.T) {
	fields := hrRepairExtractFields78()
	byName := map[string]types.ExtractFieldConfig{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	for _, want := range []string{"labor_items", "part_items", "extra_items"} {
		f, ok := byName[want]
		require.True(t, ok, "提取规则必须含 %s", want)
		require.Equal(t, "items", f.Type)
	}
	// 主表关键金额字段必须有正则兜底
	for _, want := range []string{"工时费", "材料费", "实际应收", "车架号"} {
		require.NotEmpty(t, byName[want].Regex, "%s 必须有正则兜底", want)
	}
	require.Contains(t, hrRepairPrompt78, "labor_items")
	require.Contains(t, hrMaintenancePrompt78, "No.")
	require.Contains(t, hrMaintenancePrompt78, "质保里程")
}

// 附件标签白名单与前端下拉同源。
func TestRound78AttachmentTags(t *testing.T) {
	require.True(t, fleetAttachmentTagAllowed("竣工检测单"))
	require.True(t, fleetAttachmentTagAllowed("过程照片"))
	require.False(t, fleetAttachmentTagAllowed("随便写"))
	require.False(t, strings.Contains(strings.Join(fleetAttachmentTags, ","), "—"))
}
