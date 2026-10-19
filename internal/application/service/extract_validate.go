package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是提取中台的**质量闭环**：确定性校验 + 一次自愈重试。
//
// 设计取舍：置信度不用"让模型自评分数"那种做法——自评不可靠、且要多花一轮 token。
// 这里用**确定性信号**：字段是否缺失、日期能否归一化、金额能否对上。
// 校验失败不丢弃已提取的数据（那比空结果更糟），而是带着问题清单让模型修一次，
// 修完问题更少才采用，并把问题清单记进日志用于观测。

// ExtractFieldIssue 一个字段级问题。Index 为发票在结果中的序号，-1 表示结果级问题。
type ExtractFieldIssue struct {
	Index    int    `json:"index"`
	Field    string `json:"field"`
	Kind     string `json:"kind"` // missing | format | mismatch
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

const (
	issueSeverityError = "error"
	issueSeverityWarn  = "warn"
)

// 日期形态：2026-09-02 / 2026/09/02 / 2026.09.02 / 2026年9月2日 / 20260902 / 2026-9-2
var (
	reDateCN      = regexp.MustCompile(`(\d{4})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})`)
	reDateSep     = regexp.MustCompile(`(\d{4})\s*[-/.]\s*(\d{1,2})\s*[-/.]\s*(\d{1,2})`)
	reDateCompact = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`)
)

// normalizeDateString 把常见日期写法归一为 YYYY-MM-DD；无法识别时返回原串与 false。
// 模型输出日期的形态很不稳定（实测有 20260902、2026年09月02日、2026/09/02 等），
// 不做归一化就无法排序、筛选和按期统计。
func normalizeDateString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return s, false
	}
	var y, m, d string
	if g := reDateCN.FindStringSubmatch(s); g != nil {
		y, m, d = g[1], g[2], g[3]
	} else if g := reDateSep.FindStringSubmatch(s); g != nil {
		y, m, d = g[1], g[2], g[3]
	} else if g := reDateCompact.FindStringSubmatch(s); g != nil {
		y, m, d = g[1], g[2], g[3]
	} else {
		return s, false
	}
	mi, di := atoi2(m), atoi2(d)
	if mi < 1 || mi > 12 || di < 1 || di > 31 {
		return s, false
	}
	return fmt.Sprintf("%s-%02d-%02d", y, mi, di), true
}

func atoi2(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// floatEquals 金额比较容差：分位对齐 + 相对误差，避免浮点噪声与模型四舍五入误报。
func floatEquals(a, b float64) bool {
	if math.Abs(a-b) <= 0.02 {
		return true
	}
	scale := math.Abs(a)
	if math.Abs(b) > scale {
		scale = math.Abs(b)
	}
	return scale > 0 && math.Abs(a-b)/scale <= 0.01
}

// validateInvoiceExtraction 对提取结果做确定性校验，并**就地归一化日期**。
// 返回问题清单（空切片表示无问题）。kind 非 invoice 时不做业务校验。
func validateInvoiceExtraction(res *InvoiceExtractionResult, cfg *types.KbExtractConfig) []ExtractFieldIssue {
	issues := make([]ExtractFieldIssue, 0)
	if res == nil {
		return issues
	}
	if res.Kind != "invoice" {
		return issues
	}
	// 用户配置的必填字段（ExtractFieldConfig.Required）。只对**标准契约键**生效：
	// 配置里可能写中文名（"车牌号"）或明细子列，那类字段的判定不在本校验器职责内，
	// 强行映射会误报。规则 2/3 已覆盖的键不再重复报（去重见 hasIssueForField）。
	var requiredFields []string
	if cfg != nil {
		for _, f := range cfg.Fields {
			if !f.Enabled || !f.Required {
				continue
			}
			if n := strings.TrimSpace(f.Name); n != "" {
				requiredFields = append(requiredFields, n)
			}
		}
	}
	for i := range res.Invoices {
		inv := &res.Invoices[i]

		// 1) 日期归一化：能归一就改写，不能归一则记 format 问题（warn）
		if inv.InvoiceDate != "" {
			if norm, ok := normalizeDateString(inv.InvoiceDate); ok {
				if norm != inv.InvoiceDate {
					inv.InvoiceDate = norm
				}
			} else {
				issues = append(issues, ExtractFieldIssue{
					Index: i, Field: "invoice_date", Kind: "format",
					Detail:   fmt.Sprintf("日期无法归一为 YYYY-MM-DD: %q", inv.InvoiceDate),
					Severity: issueSeverityWarn,
				})
			}
		} else {
			issues = append(issues, ExtractFieldIssue{
				Index: i, Field: "invoice_date", Kind: "missing",
				Detail: "缺少开票日期", Severity: issueSeverityWarn,
			})
		}

		// 2) 标识：发票号与发票代码至少要有一个，否则列表无法识别/查重
		if strings.TrimSpace(inv.InvoiceNo) == "" && strings.TrimSpace(inv.InvoiceCode) == "" {
			issues = append(issues, ExtractFieldIssue{
				Index: i, Field: "invoice_no", Kind: "missing",
				Detail: "缺少发票号码与发票代码（无法标识该票据）", Severity: issueSeverityError,
			})
		}

		// 3) 金额：价税合计与不含税金额至少要有一个
		if inv.TotalAmount == nil && inv.Amount == nil {
			issues = append(issues, ExtractFieldIssue{
				Index: i, Field: "total_amount", Kind: "missing",
				Detail: "缺少金额（价税合计与不含税金额均为空）", Severity: issueSeverityError,
			})
		}

		// 4) 交叉校验：明细行金额合计 应等于 不含税金额
		var lineSum float64
		var hasLine bool
		for _, l := range inv.Items {
			if l.Amount != nil {
				lineSum += *l.Amount
				hasLine = true
			}
		}
		if hasLine && inv.Amount != nil && !floatEquals(lineSum, *inv.Amount) {
			issues = append(issues, ExtractFieldIssue{
				Index: i, Field: "amount", Kind: "mismatch",
				Detail:   fmt.Sprintf("明细合计 %.2f 与不含税金额 %.2f 不一致", lineSum, *inv.Amount),
				Severity: issueSeverityWarn,
			})
		}

		// 5) 交叉校验：不含税金额 + 税额 ≈ 价税合计
		if inv.Amount != nil && inv.Tax != nil && inv.TotalAmount != nil {
			if !floatEquals(*inv.Amount+*inv.Tax, *inv.TotalAmount) {
				issues = append(issues, ExtractFieldIssue{
					Index: i, Field: "total_amount", Kind: "mismatch",
					Detail:   fmt.Sprintf("金额 %.2f + 税额 %.2f ≠ 价税合计 %.2f", *inv.Amount, *inv.Tax, *inv.TotalAmount),
					Severity: issueSeverityWarn,
				})
			}
		}

		// 6) 用户配置的必填字段为空 → error（此前 Required 只是个没人读的字段）。
		//    放在最后：跳过已被规则 1~3 报过的键，避免同一缺失被记两次。
		for _, f := range requiredFields {
			// 配置键可能是 camelCase（invoiceNo）：record 视图的 resolveField 已做
			// canonical 归一解析。结构体里没有这个 json 字段（用户配了结构体装不下
			// 的自定义 key）→ isEmpty 第二返回 false，继续跳过，与旧 accessor 判定
			// 口径一致；issue.Field 保留用户配置原名，前端才能按同一 key 高亮。
			empty, exists := newGenericRecord(i, inv).isEmpty(f)
			if !exists || !empty || hasIssueForField(issues, i, f) {
				continue
			}
			issues = append(issues, ExtractFieldIssue{
				Index: i, Field: f, Kind: "missing",
				Detail: "用户配置为必填，但未提取到该字段", Severity: issueSeverityError,
			})
		}
	}
	return issues
}

// hasIssueForField 判断某张发票的某个字段是否已被记过问题（避免重复告警）。
// 比对走 canonical 归一：规则 1~3 记的是契约键（invoice_no），用户配置可能写 invoiceNo，
// 字面比对会让同一个缺失被记两次。
func hasIssueForField(issues []ExtractFieldIssue, index int, field string) bool {
	want := canonicalFieldKey(field)
	for _, is := range issues {
		if is.Index != index {
			continue
		}
		if is.Field == field || canonicalFieldKey(is.Field) == want {
			return true
		}
	}
	return false
}

func countIssues(issues []ExtractFieldIssue, severity string) int {
	n := 0
	for _, it := range issues {
		if it.Severity == severity {
			n++
		}
	}
	return n
}

// RepairExtractAttempts 自愈用的尝试序列：非流式 → 非流式+schema。
// 自愈是"补一轮"，不是主流程，控制在 2 次以内避免 token 翻倍。
func RepairExtractAttempts(schema json.RawMessage) []ExtractAttempt {
	return []ExtractAttempt{
		{Label: "repair-nonstream"},
		{Label: "repair-nonstream-jsonobject", Schema: schema},
	}
}

// repairInvoiceExtraction 带着问题清单让模型修正一次。
// 返回修正后的结果与其问题清单；仅当 error 级问题严格减少时才应被采用（由调用方判断）。
func repairInvoiceExtraction(
	ctx context.Context,
	model chat.Chat,
	baseMessages []chat.Message,
	baseOpts *chat.ChatOptions,
	schema json.RawMessage,
	cfg *types.KbExtractConfig,
	previousRaw string,
	issues []ExtractFieldIssue,
) (*InvoiceExtractionResult, []ExtractFieldIssue, error) {
	var sb strings.Builder
	sb.WriteString("上一次提取结果存在以下问题，请修正后重新输出完整 JSON（结构与字段键名保持不变）：\n")
	for _, it := range issues {
		if it.Severity != issueSeverityError {
			continue
		}
		sb.WriteString(fmt.Sprintf("- 第 %d 张票据的字段 %s：%s（%s）\n", it.Index+1, it.Field, it.Detail, it.Kind))
	}
	sb.WriteString("\n只输出 JSON，不要解释。")

	// 上一次的完整输出可能很长（多张发票），连同原始文档一起容易顶到上下文上限，
	// 截断后再作为 assistant 轮次回灌：自愈只需要知道上次填了什么。
	prev := previousRaw
	if len([]rune(prev)) > 8000 {
		prev = string([]rune(prev)[:8000]) + "\n... (truncated)"
	}

	messages := make([]chat.Message, 0, len(baseMessages)+2)
	messages = append(messages, baseMessages...)
	messages = append(messages,
		chat.Message{Role: "assistant", Content: prev},
		chat.Message{Role: "user", Content: sb.String()},
	)

	raw, err := RunExtractWithRetry(ctx, model, messages, baseOpts,
		RepairExtractAttempts(schema),
		func(raw string) bool {
			_, perr := parseInvoiceRuleReply(raw, cfg)
			return perr == nil
		},
		func(lastRaw string, lastErr error) {
			logger.Warnf(ctx, "invoice repair attempt unparseable: %v", lastErr)
		})
	if err != nil {
		return nil, nil, err
	}
	parsed, perr := parseInvoiceRuleReply(raw, cfg)
	if perr != nil {
		return nil, nil, perr
	}
	fixedIssues := validateInvoiceExtraction(parsed, cfg)
	return parsed, fixedIssues, nil
}

// logExtractIssues 把校验问题写入日志，用于观测提取质量（不做阻断）。
func logExtractIssues(ctx context.Context, issues []ExtractFieldIssue) {
	if len(issues) == 0 {
		return
	}
	for _, it := range issues {
		logger.Warnf(ctx, "extract issue [%s] record#%d field=%s kind=%s: %s",
			it.Severity, it.Index, it.Field, it.Kind, it.Detail)
	}
}

// FinalizeInvoiceRuleResult 对**前置规则命中**（Line / MarkdownTable / FlatLayout）的
// 发票结果做与 LLM 路径完全一致的收尾：
//  1. 确定性校验（日期归一化为 YYYY-MM-DD + 发票专属交叉校验 + 配置必填）；
//  2. 后置字段级正则补洞（零 token 成本，**不触发 LLM Repair**——规则路径本来就是
//     为了省掉模型调用才存在的）；
//  3. 补洞后再校验一次（补洞可能消掉 error 级问题，清单必须准确）。
//
// 返回并写回 res.ExtractIssues；PatchedByRegex / RegexPatches 由补洞器就地写入。
// 修复的是"同一个字段，走规则路径就没有任何质量元数据、日期格式还各不相同"的不一致。
func FinalizeInvoiceRuleResult(
	ctx context.Context,
	res *InvoiceExtractionResult,
	text string,
	cfg *types.KbExtractConfig,
) []ExtractFieldIssue {
	if res == nil {
		return nil
	}
	issues := validateInvoiceExtraction(res, cfg)
	if patches := RegexFallbackInvoiceFields(res, text, cfg); len(patches) > 0 {
		logger.Infof(ctx, "invoice regex fallback patched %d field(s): %v", len(patches), patchFieldNames(patches))
		issues = validateInvoiceExtraction(res, cfg)
	}
	res.ExtractIssues = issues
	return issues
}

// ExtractValidationIssues 暴露给上层（handler）用于把质量问题回传给前端。
// 当前仅由服务内部使用；导出是为了后续接可观测面板时不必再改签名。
func ExtractValidationIssues(res *InvoiceExtractionResult, cfg *types.KbExtractConfig) []ExtractFieldIssue {
	return validateInvoiceExtraction(res, cfg)
}
