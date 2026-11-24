// 第 66 轮：社保记录「分险种 + 分缴纳方」精准导入。
//
// 致命 Bug（本轮修复）：旧实现在导入事务里执行
//
//	DELETE FROM hr_si_records WHERE tenant_id=? AND record_type=? AND source='import'
//
// 条件里既没有 bill_month 也没有 scheme —— 导入一个「工伤」文件，
// 会把**同类型（正常/补退）下所有月份、所有险种**的导入记录全部删掉。
// 真机表现：用户依次导入养老、医疗、工伤 9 个文件，最后只剩最后一个险种。
//
// 正确口径：社保导出文件天然按「单险种 + 缴纳方（单位/个人）」拆分，
// 因此覆盖的粒度必须是切片 `(bill_month, record_type, scheme, payer_side)`：
//   · 上传「养老-个人」只覆盖养老的个人侧，绝不动养老的单位侧，
//     更不动医疗/工伤/失业的任何数据；
//   · 同一 `(人, 险种)` 的「单位 + 个人」两侧仍合并存一行（既有模型不变）。
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// timeNow66 抽出来便于单测替换（冲突预检要用同一个月份解析）。
func timeNow66() time.Time { return time.Now() }

// hrSISlice66 导入切片：一个文件精确命中的覆盖范围。
type hrSISlice66 struct {
	Month  string
	Kind   string // normal / abnormal
	Scheme string
	Part   string // unit / personal
}

func (s hrSISlice66) label() string {
	side := "单位缴纳"
	if s.Part == HRPartPersonal {
		side = "个人缴纳"
	}
	return s.Scheme + " - " + side
}

// hrSIImportStats66 导入结果统计（对外回传，前端据此弹一条精准提示）。
type hrSIImportStats66 struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Removed   int `json:"removed"`
	Kept      int `json:"kept"`
	Schemes   int `json:"schemes"`
	Conflicts int `json:"conflicts"`
}

// hrSIConflict66 预检发现的一条重复切片。
type hrSIConflict66 struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// hrSICheckConflicts66 预检：按切片检查该月同类型下是否已有记录。
//
// 只要某切片已有存量数据，就视为「可能重复导入」，由前端弹确认框；
// 用户确认后带 overwrite_scope=slice 再次提交，才会真正覆盖该切片。
func (h *HRHandler) hrSICheckConflicts66(ctx context.Context, tid int64, slices []hrSISlice66) []hrSIConflict66 {
	out := make([]hrSIConflict66, 0, len(slices))
	seen := map[string]bool{}
	for _, s := range slices {
		if seen[s.label()+"|"+s.Month] {
			continue
		}
		seen[s.label()+"|"+s.Month] = true
		var cnt int64
		_ = h.db.WithContext(ctx).Model(&HRSIRecord{}).
			Where("tenant_id = ? AND bill_month = ? AND record_type = ? AND scheme = ? AND source = ?",
				tid, s.Month, s.Kind, s.Scheme, "import").
			Count(&cnt).Error
		if cnt == 0 {
			continue
		}
		out = append(out, hrSIConflict66{Label: s.label(), Count: int(cnt)})
	}
	return out
}

// hrSIApplyImport66 在同一事务里「按切片覆盖 + 按 (人,险种) 合并更新」。
//
// 不做整月 DELETE；只把本次上传精确命中的那一侧金额清零后重写。
func (h *HRHandler) hrSIApplyImport66(
	ctx context.Context, tx *gorm.DB, tid int64,
	slices []hrSISlice66, items []HRSIRecord, pageSides map[string]map[string]bool,
) (hrSIImportStats66, error) {
	stats := hrSIImportStats66{}

	// ① 按切片把对应侧的金额清零（保留另一侧）
	for _, s := range slices {
		var existing []HRSIRecord
		if err := tx.Where("tenant_id = ? AND bill_month = ? AND record_type = ? AND scheme = ? AND source = ?",
			tid, s.Month, s.Kind, s.Scheme, "import").Find(&existing).Error; err != nil {
			return stats, err
		}
		for i := range existing {
			row := &existing[i]
			if s.Part == HRPartPersonal {
				row.PersonalAmount = 0
				row.PersonalRate = ""
			} else {
				row.UnitAmount = 0
				row.UnitRate = ""
			}
			row.TotalAmount = round2(row.UnitAmount + row.PersonalAmount)
			if row.TotalAmount == 0 && row.UnitRate == "" && row.PersonalRate == "" {
				// 两侧都被清空 → 该人该险种已无数据，直接移除（避免 0 元幽灵行）
				if err := tx.Delete(&HRSIRecord{}, "id = ?", row.ID).Error; err != nil {
					return stats, err
				}
				stats.Removed++
				continue
			}
			if err := tx.Save(row).Error; err != nil {
				return stats, err
			}
		}
	}

	// ② 按 (身份证, 险种, 月份, 类型) 合并写入
	schemes := map[string]bool{}
	for i := range items {
		it := &items[i]
		schemes[it.Scheme] = true
		var hit HRSIRecord
		err := tx.Where("tenant_id = ? AND bill_month = ? AND record_type = ? AND scheme = ? AND id_number = ?",
			tid, it.BillMonth, it.RecordType, it.Scheme, it.IDNumber).First(&hit).Error
		if err == gorm.ErrRecordNotFound {
			if err := tx.Create(it).Error; err != nil {
				return stats, err
			}
			stats.Created++
			continue
		}
		if err != nil {
			return stats, err
		}
		// 只更新本次上传触及的那一侧，另一侧保留原值
		sides := pageSides[it.IDNumber+"|"+it.Scheme]
		if sides[HRPartPersonal] {
			hit.PersonalAmount = it.PersonalAmount
			hit.PersonalRate = it.PersonalRate
		}
		if sides[HRPartUnit] {
			hit.UnitAmount = it.UnitAmount
			hit.UnitRate = it.UnitRate
		}
		hit.TotalAmount = round2(hit.UnitAmount + hit.PersonalAmount)
		hit.Base = it.Base
		hit.PersonCode = it.PersonCode
		hit.Name = it.Name
		hit.IDNumber = it.IDNumber
		hit.BillMonth = it.BillMonth
		hit.Period = it.Period
		hit.RecordType = it.RecordType
		hit.Scheme = it.Scheme
		hit.SourceFile = it.SourceFile
		hit.Source = "import"
		hit.CustomData = it.CustomData
		if err := tx.Save(&hit).Error; err != nil {
			return stats, err
		}
		stats.Updated++
	}

	stats.Schemes = len(schemes)
	// 当前月累计保留（含未触及的其他险种）
	var kept int64
	// 第 68 轮：kept 必须同月统计 —— 旧实现只按 record_type 计数，
	// 会把其它月份的行算进来，用户看到的「保留 N 条」与当月实际不符。
	_ = tx.Model(&HRSIRecord{}).
		Where("tenant_id = ? AND bill_month = ? AND record_type = ? AND source = ?",
			tid, items[0].BillMonth, items[0].RecordType, "import").
		Count(&kept).Error
	stats.Kept = int(kept)
	return stats, nil
}

// CheckSIRecordsImport POST /hr/social/records/import-check
//
// 上传同一批文件后先问一句「这些切片已经有数据了，要覆盖吗？」——
// 前端据此弹确认框。返回的 conflicts 为空表示可以直接导入。
func (h *HRHandler) CheckSIRecordsImport(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	form, err := c.MultipartForm()
	if err != nil {
		hrFail(c, "读取上传文件失败: "+err.Error())
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		hrFail(c, "没有上传任何文件")
		return
	}
	kind := strings.TrimSpace(c.PostForm("record_type"))
	if kind != "abnormal" {
		kind = "normal"
	}
	explicitMonth := strings.TrimSpace(c.PostForm("bill_month"))
	if explicitMonth == "" {
		explicitMonth = strings.TrimSpace(c.PostForm("month"))
	}
	if explicitMonth != "" {
		ym, e := hrNormalizeMonthLabel(explicitMonth)
		if e != nil {
			hrFail(c, "账单月份必须为 YYYY-MM")
			return
		}
		explicitMonth = ym
	}
	var slices []hrSISlice66
	for _, fh := range files {
		meta := hrSIParseFileName(fh.Filename)
		scheme := hrSISchemeOfFile(fh.Filename)
		if strings.TrimSpace(scheme) == "" {
			continue
		}
		slices = append(slices, hrSISlice66{
			Month: hrResolveImportMonth(explicitMonth, meta.YearMonth, timeNow66()),
			Kind:  kind, Scheme: scheme, Part: meta.Part,
		})
	}
	conflicts := h.hrSICheckConflicts66(ctx, tid, slices)
	hrOK(c, gin.H{"conflicts": conflicts, "checked": len(slices)})
}
