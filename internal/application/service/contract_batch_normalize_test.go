package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 合同/制度的批量构建必须与发票共用同一套取数与归一：
//  1. 只取 text / image_ocr / image_caption（ChunksForExtraction）；
//  2. 裸 HTML 表格归一成模型可读文本（否则整张表被读成噪声）；
//  3. 有物理页码时插入 [Page N] 锚点，便于按页归位。
func TestContractBatchNormalizesHTMLAndKeepsPageMarker(t *testing.T) {
	chunks := []*types.Chunk{
		{
			ChunkIndex: 0,
			ChunkType:  types.ChunkTypeText,
			Content: "合同正文\n<table><tr><td>甲方</td><td>乙方</td></tr>" +
				"<tr><td>重庆星达铜业有限公司</td><td>某某公司</td></tr></table>",
			SourceLocators: []types.SourceLocator{{Type: "pdf", Page: 3}},
		},
		{ChunkIndex: 1, ChunkType: types.ChunkTypeSummary, Content: "摘要不应进入提取"},
		{ChunkIndex: 2, ChunkType: types.ChunkTypeText, Content: "   "},
	}
	batches := BuildContractExtractionBatches("合同A.pdf", "", chunks, 0)
	require.NotEmpty(t, batches)
	text := strings.Join(batches, "\n")

	assert.Contains(t, text, "合同正文")
	assert.Contains(t, text, "重庆星达铜业有限公司")
	assert.Contains(t, text, "[Page 3]", "有物理页码时必须插入页锚点")
	assert.NotContains(t, text, "<td>", "HTML 表格必须被归一，不能原样喂模型")
	assert.NotContains(t, text, "摘要不应进入提取", "summary chunk 不得进入提取")
}
