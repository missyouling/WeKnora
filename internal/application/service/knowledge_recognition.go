package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// GetRecognitionConfig returns the KB-level document recognition rules.
// KBs without a stored config fall back to module defaults (invoice: common
// include rule + built-in keyword type rules + 5-type enum; contract: common
// include rule, empty type rules). When the stored type list is empty, the
// contract type list auto-fills from existing contracts so the rule panel and
// the list filter always have something to choose from.
func (s *knowledgeService) GetRecognitionConfig(ctx context.Context, kbID string) (*types.RecognitionConfig, error) {
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return nil, err
	}
	cfg := kb.RecognitionConfig
	if cfg == nil {
		cfg = types.DefaultContractRecognitionConfig()
		// 发票知识库的默认配置含内置类型归类规则与 5 枚举。
		// 制度知识库的默认配置含内置类型归类规则与 6 枚举。
		// 奖惩知识库的默认配置含内置类型归类规则与 4 枚举。
		// 判断方式：KB 名含"发票"/"制度"/"奖惩"（向导创建时命名固定）或识别配置已存在。
		if strings.Contains(kb.Name, "发票") {
			cfg = types.DefaultInvoiceRecognitionConfig()
		} else if strings.Contains(kb.Name, "制度") {
			cfg = types.DefaultRegulationRecognitionConfig()
		} else if strings.Contains(kb.Name, "奖惩") {
			cfg = types.DefaultAwardPunishRecognitionConfig()
		}
	}
	// 分类列表为空时自动填充（发票：5 枚举；合同：现有合同类型）；非空时也
	// 合并现有类型（后续新出现的类型也能选到）。均不持久化。
	merged := make([]string, 0, len(cfg.Types)+8)
	seen := map[string]struct{}{}
	for _, t := range cfg.Types {
		if t != "" {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				merged = append(merged, t)
			}
		}
	}
	for _, t := range s.existingTypesFor(ctx, kb) {
		if t != "" {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				merged = append(merged, t)
			}
		}
	}
	cfg.Types = merged
	// 奖惩知识库措施库为空时合并默认预置措施（不持久化），
	// 保证抽屉措施多选与措施管理面板始终有可选内容。
	if strings.Contains(kb.Name, "奖惩") {
		defaults := types.DefaultAwardPunishRecognitionConfig().Measures
		names := map[string]struct{}{}
		for _, m := range cfg.Measures {
			if m.Name != "" {
				names[m.Name] = struct{}{}
			}
		}
		for _, m := range defaults {
			if m.Name == "" {
				continue
			}
			if _, ok := names[m.Name]; !ok {
				cfg.Measures = append(cfg.Measures, m)
			}
		}
	}
	return cfg, nil
}

// existingTypesFor returns the module type list for a KB: the invoice enum for
// invoice KBs, the regulation enum for regulation KBs, the award/punish enum
// for award/punish KBs, or the distinct contract types seen in the KB for
// contract KBs.
func (s *knowledgeService) existingTypesFor(ctx context.Context, kb *types.KnowledgeBase) []string {
	if strings.Contains(kb.Name, "发票") {
		return []string{"专用发票", "普通发票", "医疗收据", "财政收据", "其它票据"}
	}
	if strings.Contains(kb.Name, "制度") {
		return []string{"人事管理", "财务管理", "生产管理", "行政管理", "安全管理", "其它制度"}
	}
	if strings.Contains(kb.Name, "奖惩") {
		return []string{"处罚", "奖励", "通报", "其它奖惩"}
	}
	cts, err := s.ListContractTypes(ctx, kb.ID)
	if err != nil || len(cts) == 0 {
		return nil
	}
	out := make([]string, 0, len(cts))
	for _, c := range cts {
		if c.ContractType != "" {
			out = append(out, c.ContractType)
		}
	}
	return out
}

// SaveRecognitionConfig persists the KB-level recognition rules. Type rules
// whose target type is not in the (explicit or auto-filled) type list are
// rejected, so rules never point at a removed category.
func (s *knowledgeService) SaveRecognitionConfig(ctx context.Context, kbID string, cfg *types.RecognitionConfig) error {
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &types.RecognitionConfig{Enabled: false}
	}
	// 校验类型归类规则指向的分类：合并显式 types + 自动填充现有分类
	allowed := map[string]struct{}{}
	for _, t := range cfg.Types {
		if t != "" {
			allowed[t] = struct{}{}
		}
	}
	for _, t := range s.existingTypesFor(ctx, kb) {
		allowed[t] = struct{}{}
	}
	for _, r := range cfg.TypeRules {
		if r.Enabled && r.Type != "" {
			if _, ok := allowed[r.Type]; !ok {
				return fmt.Errorf("归类目标「%s」不存在于分类列表，请先添加该分类", r.Type)
			}
		}
	}
	_, err = s.kbService.UpdateKnowledgeBase(ctx, kb.ID, kb.Name, kb.Description, &types.KnowledgeBaseConfig{
		RecognitionConfig: cfg,
	})
	return err
}

// MatchIncludeRules reports whether any enabled include-rule matches the text.
// A rule matches when (keyword mode) every keyword (AND) or any keyword (OR) is
// contained in the text, or (regex mode) the regex finds a match. Empty rules
// never match. A disabled config (Enabled=false) never matches.
func MatchIncludeRules(text string, cfg *types.RecognitionConfig) bool {
	if cfg == nil || !cfg.Enabled || strings.TrimSpace(text) == "" {
		return false
	}
	for _, r := range cfg.IncludeRules {
		if !r.Enabled {
			continue
		}
		switch r.MatchType {
		case "regex":
			if r.Regex == "" {
				continue
			}
			if re, err := regexp.Compile(r.Regex); err == nil && re.MatchString(text) {
				return true
			}
		default: // keyword
			if len(r.Keywords) == 0 {
				continue
			}
			if strings.EqualFold(r.Logic, "AND") {
				all := true
				for _, kw := range r.Keywords {
					if kw == "" || !strings.Contains(text, kw) {
						all = false
						break
					}
				}
				if all {
					return true
				}
			} else { // OR
				for _, kw := range r.Keywords {
					if kw != "" && strings.Contains(text, kw) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ClassifyTypeFromRules returns the type of the first enabled type-classify rule
// (ordered by Priority asc) whose pattern matches the text; "" when none hit.
func ClassifyTypeFromRules(text string, cfg *types.RecognitionConfig) string {
	if cfg == nil || !cfg.Enabled || strings.TrimSpace(text) == "" {
		return ""
	}
	type rule struct {
		r *types.TypeClassifyRule
	}
	rules := make([]rule, 0, len(cfg.TypeRules))
	for i := range cfg.TypeRules {
		if cfg.TypeRules[i].Enabled {
			rules = append(rules, rule{r: &cfg.TypeRules[i]})
		}
	}
	// stable insertion-ordered by priority asc
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].r.Priority < rules[i].r.Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
	for _, rr := range rules {
		p := rr.r.Pattern
		if p == "" {
			continue
		}
		if rr.r.IsRegex {
			if re, err := regexp.Compile(p); err == nil && re.MatchString(text) {
				return rr.r.Type
			}
		} else if strings.Contains(text, p) {
			return rr.r.Type
		}
	}
	return ""
}

// ReassessRecognition re-runs the include-judgement rules against the retained
// auto-deleted rows of the KB. Rows whose retained file text hits an enabled
// rule are restored as manual (待补录) records via RestoreDeletedKnowledge, so
// they re-appear in the module list for the user to edit. Already-restored or
// permanently purged rows are untouched. Returns the number of rows restored.
func (s *knowledgeService) ReassessRecognition(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	cfg, err := s.GetRecognitionConfig(ctx, kbID)
	if err != nil {
		return 0, err
	}
	if cfg == nil || !cfg.Enabled || len(cfg.IncludeRules) == 0 {
		logger.Infof(ctx, "ReassessRecognition skipped: no enabled include rules for KB %s", kbID)
		return 0, nil
	}
	restored := 0
	page := 1
	for {
		items, total, lerr := s.repo.ListDeletedKnowledge(ctx, tenantID, kbID, page, 200, "")
		if lerr != nil {
			return restored, lerr
		}
		for _, k := range items {
			if k == nil {
				continue
			}
			text := s.knowledgeTextForRules(ctx, k)
			if text == "" {
				continue
			}
			if MatchIncludeRules(text, cfg) {
				if _, rerr := s.RestoreDeletedKnowledge(ctx, k.ID); rerr != nil {
					logger.Warnf(ctx, "ReassessRecognition restore %s failed: %v", k.ID, rerr)
					continue
				}
				restored++
				logger.Infof(ctx, "ReassessRecognition restored %s (file=%s) hit by include rule", k.ID, k.FileName)
			}
		}
		if int64(page)*200 >= total {
			break
		}
		page++
	}
	logger.Infof(ctx, "ReassessRecognition done for KB %s: %d rows restored", kbID, restored)
	return restored, nil
}

// knowledgeTextForRules reconstructs the full retained text of a knowledge row
// from its chunks, used by rule matching (scanned documents have no text).
func (s *knowledgeService) knowledgeTextForRules(ctx context.Context, k *types.Knowledge) string {
	chunks, err := s.chunkService.ListChunksByKnowledgeID(ctx, k.ID)
	if err != nil || len(chunks) == 0 {
		return k.Description
	}
	var b strings.Builder
	for _, c := range chunks {
		if c != nil && strings.TrimSpace(c.Content) != "" {
			b.WriteString(c.Content)
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		return k.Description
	}
	return b.String()
}
