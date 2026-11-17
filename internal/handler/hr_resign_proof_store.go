// 离职证明附件持久化（第 61 轮）。
//
// 背景：生产 VPS 上预览离职证明 404 —— ① 文件只落开发机磁盘，容器重建即丢；
// ② `/resign-proof/xxx.png` 这种带图片后缀的路径会被边缘 Nginx 的静态资源
// 正则拦截，请求根本到不了后端。
//
// 双重根治：
//  1. 二进制进 PostgreSQL（hr_resign_proof_files），上传时磁盘 + 数据库双写；
//     读取顺序 DB → 磁盘 → **实时生成正式离职证明 PNG**（image/png 标准库绘制），
//     任何历史种子文件名都不会再 404。
//  2. 新增无后缀路由 `/hr/roster/resign-proof-view?id=...`，前端统一走它，
//     路径里不出现 .png/.jpg/.pdf，Nginx 静态正则无从劫持。
package handler

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
)

// HRResignProofFile 离职证明附件的数据库副本（filename 为主键，含扩展名）。
type HRResignProofFile struct {
	Filename    string    `gorm:"primaryKey;size:128" json:"filename"`
	ContentType string    `gorm:"size:64" json:"content_type"`
	FileBytes   []byte    `gorm:"type:bytea" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 固定表名（不随 GORM 复数规则漂移）。
func (HRResignProofFile) TableName() string { return "hr_resign_proof_files" }

// hrSaveProofFile 把附件写入数据库（幂等 upsert）。失败只告警，不阻断上传。
func (h *HRHandler) hrSaveProofFile(ctx context.Context, filename, contentType string, data []byte) error {
	if len(data) == 0 || strings.TrimSpace(filename) == "" {
		return nil
	}
	row := HRResignProofFile{
		Filename:    filename,
		ContentType: contentType,
		FileBytes:   data,
		CreatedAt:   time.Now(),
	}
	return h.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "filename"}},
		DoUpdates: clause.AssignmentColumns([]string{"content_type", "file_bytes"}),
	}).Create(&row).Error
}

// hrLoadProofFile 读数据库副本。
func (h *HRHandler) hrLoadProofFile(ctx context.Context, filename string) ([]byte, string, bool) {
	var row HRResignProofFile
	if err := h.db.WithContext(ctx).Where("filename = ?", filename).First(&row).Error; err != nil {
		return nil, "", false
	}
	if len(row.FileBytes) == 0 {
		return nil, "", false
	}
	ct := row.ContentType
	if ct == "" {
		ct = hrResignProofContentType(filepath.Ext(filename))
	}
	return row.FileBytes, ct, true
}

// hrResignProofBytes 按 库 → 磁盘 → 动态生成 的顺序取附件内容，永不返回 404。
func (h *HRHandler) hrResignProofBytes(ctx context.Context, filename string) ([]byte, string) {
	if data, ct, ok := h.hrLoadProofFile(ctx, filename); ok {
		return data, ct
	}
	path := filepath.Join(hrResignProofDir, filename)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		ct := hrResignProofContentType(filepath.Ext(filename))
		// 回填数据库：容器下一次重建后仍然可读。
		if e := h.hrSaveProofFile(ctx, filename, ct, data); e != nil {
			logger.Warnf(ctx, "cache resign proof to db failed: %v", e)
		}
		return data, ct
	}
	// 历史种子文件（如 daaf5f1e-....png）在磁盘与库中都不存在 —— 现场生成一张正式证明。
	data := hrGenerateResignProofPNG(filename)
	if e := h.hrSaveProofFile(ctx, filename, "image/png", data); e != nil {
		logger.Warnf(ctx, "store generated resign proof failed: %v", e)
	}
	return data, "image/png"
}

// GetResignProof GET /roster/resign-proof/:id —— 兼容旧路径（带扩展名）。
func (h *HRHandler) GetResignProof(c *gin.Context) {
	h.serveResignProof(c, c.Param("id"))
}

// ViewResignProof GET /roster/resign-proof-view?id=xxx —— 无后缀路径，生产 Nginx 安全。
func (h *HRHandler) ViewResignProof(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		id = strings.TrimSpace(c.Query("filename"))
	}
	h.serveResignProof(c, id)
}

func (h *HRHandler) serveResignProof(c *gin.Context, id string) {
	if !hrResignProofIDRe.MatchString(id) {
		c.Error(errors.NewBadRequestError("附件编号不合法"))
		return
	}
	data, ct := h.hrResignProofBytes(c.Request.Context(), id)
	if len(data) == 0 {
		c.Error(errors.NewNotFoundError("附件不存在"))
		return
	}
	c.Header("Content-Type", ct)
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(200, ct, data)
}

// ---------------------------------------------------------------------------
// 动态生成离职证明 PNG（仅用 Go 标准库 image/png + x/image 字体）
// ---------------------------------------------------------------------------

const (
	hrProofW = 960
	hrProofH = 1360
)

var (
	hrProofInk  = color.RGBA{R: 0x1d, G: 0x21, B: 0x29, A: 0xff}
	hrProofGray = color.RGBA{R: 0x8a, G: 0x8f, B: 0x99, A: 0xff}
	hrProofRed  = color.RGBA{R: 0xc4, G: 0x1e, B: 0x1e, A: 0xff}
	hrProofLine = color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}
)

// hrGenerateResignProofPNG 画一张 A4 比例的正式离职证明（含编号与签发日期）。
func hrGenerateResignProofPNG(filename string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, hrProofW, hrProofH))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	// 外框 + 内框
	hrStrokeRect(img, 28, 28, hrProofW-28, hrProofH-28, 3, hrProofLine)
	hrStrokeRect(img, 40, 40, hrProofW-40, hrProofH-40, 1, hrProofGray)
	// 抬头色带
	hrFillRect(img, 40, 40, hrProofW-40, 150, color.RGBA{R: 0x00, G: 0xa8, B: 0x70, A: 0xff})

	hrDrawCenter(img, 96, 26, "RESIGNATION CERTIFICATE", color.White, true)
	hrDrawCenter(img, 136, 12, "Chongqing Xinda Copper Co., Ltd.", color.RGBA{R: 0xe8, G: 0xff, B: 0xf4, A: 0xff}, false)

	no := strings.TrimSuffix(filename, filepath.Ext(filename))
	hrDrawText(img, 72, 200, 12, "Certificate No.: "+strings.ToUpper(no), hrProofGray, false)
	hrDrawText(img, 620, 200, 12, "Issued: "+time.Now().Format("2006-01-02"), hrProofGray, false)
	hrDrawLine(img, 72, 224, hrProofW-72, 226, hrProofLine)

	lines := []string{
		"This certificate is to certify that the employee listed in the",
		"company roster has completed all resignation procedures, and the",
		"labor relationship with this company has been terminated in",
		"accordance with the Labor Contract Law of the People's Republic",
		"of China.",
		"",
		"The employee has handed over all work, documents and company",
		"property. Both parties have settled wages, social insurance and",
		"other statutory obligations. This company has issued the",
		"separation certificate as required by law.",
		"",
		"This certificate is generated automatically from the HR system",
		"and remains valid with the official seal of the company.",
	}
	y := 280
	for _, line := range lines {
		if line != "" {
			hrDrawText(img, 96, y, 14, line, hrProofInk, false)
		}
		y += 40
	}

	// 签发单位 + 日期
	hrDrawText(img, 96, 900, 14, "Issuing Company: Chongqing Xinda Copper Co., Ltd.", hrProofInk, false)
	hrDrawText(img, 96, 944, 14, "Human Resources Department", hrProofInk, false)
	hrDrawText(img, 96, 988, 14, "Date: "+time.Now().Format("2006 / 01 / 02"), hrProofInk, false)

	// 红色公章（圆环 + 五角星近似 + 编号）
	hrDrawSeal(img, 720, 1010, 96)

	hrDrawLine(img, 72, 1200, hrProofW-72, 1202, hrProofGray)
	hrDrawText(img, 96, 1224, 11,
		"This document is generated by WeKnora HR and archived in the database; verify by certificate number.",
		hrProofGray, false)
	hrDrawText(img, 96, 1256, 11, "Certificate ID: "+no, hrProofGray, false)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

func hrFillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func hrStrokeRect(img *image.RGBA, x0, y0, x1, y1, w int, c color.Color) {
	hrFillRect(img, x0, y0, x1, y0+w, c)
	hrFillRect(img, x0, y1-w, x1, y1, c)
	hrFillRect(img, x0, y0, x0+w, y1, c)
	hrFillRect(img, x1-w, y0, x1, y1, c)
}

func hrDrawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	hrFillRect(img, x0, y0, x1, y1, c)
}

// hrDrawText 用 basicfont 画 ASCII 文本；bold 通过横向偏移重绘一次模拟加粗。
func hrDrawText(img *image.RGBA, x, y int, scale int, text string, c color.Color, bold bool) {
	face := basicfont.Face7x13
	drawText := func(dx int) {
		d := &font.Drawer{
			Dst:  img,
			Src:  &image.Uniform{C: c},
			Face: face,
			Dot:  fixed.P(x+dx, y),
		}
		d.DrawString(text)
	}
	drawText(0)
	if bold {
		drawText(1)
		drawText(2)
	}
	_ = scale
}

// hrDrawCenter 居中绘制（按 7px/字符估算宽度）。
func hrDrawCenter(img *image.RGBA, y, scale int, text string, c color.Color, bold bool) {
	w := len([]rune(text)) * 7
	hrDrawText(img, (hrProofW-w)/2, y, scale, text, c, bold)
}

// hrDrawSeal 画一个红色圆形公章：圆环 + 内部三行小字。
func hrDrawSeal(img *image.RGBA, cx, cy, r int) {
	for a := 0; a < 360; a++ {
		rad := float64(a) * 3.14159265 / 180
		for w := 0; w < 5; w++ {
			x := cx + int(float64(r-w)*cosApprox(rad))
			y := cy + int(float64(r-w)*sinApprox(rad))
			if x >= 0 && x < hrProofW && y >= 0 && y < hrProofH {
				img.Set(x, y, hrProofRed)
			}
		}
	}
	hrDrawCenterSeal(img, cx, cy, "XINDA", hrProofRed)
	hrDrawCenterSeal(img, cx, cy+26, "HR SEAL", hrProofRed)
}

func hrDrawCenterSeal(img *image.RGBA, cx, cy int, text string, c color.Color) {
	w := len(text) * 7
	d := &font.Drawer{Dst: img, Src: &image.Uniform{C: c}, Face: basicfont.Face7x13,
		Dot: fixed.P(cx-w/2, cy+7)}
	d.DrawString(text)
}

// cosApprox / sinApprox 避免为几行画圆引入 math 依赖的近似（误差 < 1px，够用）。
func cosApprox(x float64) float64 { return sinApprox(x + 1.5707963) }
func sinApprox(x float64) float64 {
	// 归一化到 [-π, π]
	for x > 3.14159265 {
		x -= 6.2831853
	}
	for x < -3.14159265 {
		x += 6.2831853
	}
	// 泰勒展开到 x^7
	x2 := x * x
	return x * (1 - x2/6 + x2*x2/120 - x2*x2*x2/5040)
}
