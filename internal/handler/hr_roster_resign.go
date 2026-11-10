// 花名册离职证明附件（第 46 轮）。
//
// 文件落在工作目录下的 data/hr_resign_proofs/，用 uuid + 白名单扩展名命名；
// 读取接口对 id 做严格正则校验，杜绝目录穿越。
package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/errors"
)

const (
	hrResignProofDir     = "data/hr_resign_proofs"
	hrResignProofMaxSize = 10 << 20 // 10MB
)

var hrResignProofExts = map[string]bool{
	".pdf": true, ".jpg": true, ".jpeg": true, ".png": true,
}

// hrResignProofIDRe 只允许 uuid + 白名单扩展名，禁止任何路径分隔符。
var hrResignProofIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{36}\.(pdf|jpg|jpeg|png)$`)

func hrResignProofContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

// UploadResignProof POST /roster/resign-proof —— 上传离职证明扫描件。
func (h *HRHandler) UploadResignProof(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.Error(errors.NewBadRequestError("缺少 file 参数"))
		return
	}
	if fh.Size > hrResignProofMaxSize {
		c.Error(errors.NewBadRequestError("离职证明不能超过 10MB"))
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !hrResignProofExts[ext] {
		c.Error(errors.NewBadRequestError("离职证明仅支持 PDF / JPG / PNG 格式"))
		return
	}
	if err := os.MkdirAll(hrResignProofDir, 0o755); err != nil {
		c.Error(errors.NewInternalServerError("创建附件目录失败: " + err.Error()))
		return
	}
	id := uuid.New().String() + ext
	dst := filepath.Join(hrResignProofDir, id)
	if err := c.SaveUploadedFile(fh, dst); err != nil {
		c.Error(errors.NewInternalServerError("保存附件失败: " + err.Error()))
		return
	}
	hrOK(c, gin.H{
		"id":   id,
		"name": fh.Filename,
		"path": "/api/v1/fleet/hr/roster/resign-proof/" + id,
	})
}

// GetResignProof GET /roster/resign-proof/:id —— 在线预览 / 下载附件。
func (h *HRHandler) GetResignProof(c *gin.Context) {
	id := c.Param("id")
	if !hrResignProofIDRe.MatchString(id) {
		c.Error(errors.NewBadRequestError("附件编号不合法"))
		return
	}
	path := filepath.Join(hrResignProofDir, id)
	if _, err := os.Stat(path); err != nil {
		c.Error(errors.NewNotFoundError("附件不存在"))
		return
	}
	c.Header("Content-Type", hrResignProofContentType(filepath.Ext(id)))
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, id))
	c.File(path)
}

// hrResignProofURL 把已存的 id 还原成可访问路径（供列表/详情使用）。
func hrResignProofURL(id string) string {
	if !hrResignProofIDRe.MatchString(id) {
		return ""
	}
	return "/api/v1/fleet/hr/roster/resign-proof/" + id
}
