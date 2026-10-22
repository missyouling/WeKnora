package service

import (
	"regexp"
	"strings"
)

// 甲乙方锚点核对（2026-10-22 真机事故）。
//
// 事故：同一组厂房租赁合同，主合同提取为
//   甲方：重庆星达铜业有限公司 / 乙方：重庆持恒模具有限公司
// 而三份补充协议全部提取成
//   甲方：重庆持恒模具有限公司 / 乙方：重庆星达铜业有限公司
// 根因：补充协议正文写的是「出租方：A / 承租方：B」，没有甲乙方标签，
// 模型按主合同角色猜，猜反了；此前没有任何确定性后处理核对原文。
//
// 本文件提供证据驱动的兜底：**只认原文/摘要里明确写出来的角色标签**，
// 认不到就原样保留模型结果，绝不臆造。

// 角色标签 → 甲/乙方 的锚定正则。
//
// 覆盖三类真实形态（优先级从高到低）：
//  1. 甲方：X / 乙方：Y（含「甲方（出租方）：X」这类带角色括注）
//  2. 出租方（甲方）：X / 承租方（乙方）：Y（括注里的甲乙方才是权威标签）
//  3. 出租方：X / 承租方：Y（无甲乙方标签，按合同角色约定映射）
var (
	// partyARe / partyBRe 直接锚定甲/乙方。覆盖两种真实形态：
	//   1. 标签在前：「甲方：X」「甲方（出租方）：X」
	//   2. 公司名在前：「X（甲方）」——知识库摘要常用（"星达铜业（甲方）与持恒（乙方）签订…"）
	// Go 的 RE2 不支持负向断言，第二分支改为「以非角色字符开头」的字符类：
	// 公司名首字排除 承/出/包/租（角色词首字），避免把「出租方（甲方）」截成
	// 「出租方」当公司名。代价是公司名以这些字开头时漏匹配——概率极低，
	// 且那种情况还有第一分支兜底。
	// 第二分支（「X（甲方）」）用非贪婪 + 禁止连接词字符类收口：
	// `+?` 非贪婪避免把「该合同为」一起吞进来；字符类排除 为与和及、，
	// 因为公司名里不会出现这些字，而叙述性前缀（"该合同为""与"）一定含它们。
	// 第二分支用 `.*?` 宽松匹配，真正的收口在 anchoredParty 里做
	// （过滤含连接词的候选、取最短合法公司名）——RE2 没有负向断言，
	// 在正则里硬收口会把「出租方（甲方）」和「该合同为X（甲方）」一起误伤。
	// 三个分支，覆盖真实合同里全部标签形态：
	//   a)「甲方：X」                                  → m[1]
	//   b)「甲方（出租方）：X」（标签后括注）            → m[1]
	//   c)「出租方（甲方）：X」（标签前角色括注）        → m[2]
	//   d)「X（甲方）」（公司名在前，知识库摘要常用）      → m[3]
	// m[3] 的收口在 anchoredParty 里做（过滤连接词/角色词、取最短）。
	partyARe = regexp.MustCompile(`甲\s*方\s*(?:[（(][^）)\n]{0,12}[）)])?\s*[:：]\s*([^\s，,；;。\n]{2,60})|(?:出\s*租\s*方|承\s*租\s*方|托\s*管\s*方|发\s*包\s*方)\s*[（(]\s*甲\s*方\s*[）)]\s*[:：]\s*([^\s，,；;。\n]{2,60})|([^\s，,；;。\n]{2,60})\s*[（(]\s*甲\s*方\s*[）)]`)
	partyBRe = regexp.MustCompile(`乙\s*方\s*(?:[（(][^）)\n]{0,12}[）)])?\s*[:：]\s*([^\s，,；;。\n]{2,60})|(?:出\s*租\s*方|承\s*租\s*方|托\s*管\s*方|发\s*包\s*方)\s*[（(]\s*乙\s*方\s*[）)]\s*[:：]\s*([^\s，,；;。\n]{2,60})|([^\s，,；;。\n]{2,60})\s*[（(]\s*乙\s*方\s*[）)]`)
	// lessorRe / lesseeRe 只在**全文没有甲乙方标签**时启用的角色标签。
	lessorRe = regexp.MustCompile(`(?:出\s*租\s*方|出\s*租\s*人|托\s*管\s*方)\s*(?:[（(][^）)\n]{0,12}[）)])?\s*[:：]\s*([^\s，,；;。\n]{2,60})`)
	lesseeRe = regexp.MustCompile(`(?:承\s*租\s*方|承\s*租\s*人|承\s*包\s*方)\s*(?:[（(][^）)\n]{0,12}[）)])?\s*[:：]\s*([^\s，,；;。\n]{2,60})`)
)

// partyLabelColonRe 明确的「甲/乙方 + 冒号」标签（用于判断该文本是否自带甲乙方标签）。
// 只用带冒号的形态：正文里出现「甲方」二字（如"甲方应按时支付租金"）不算标签，
// 否则会误判为"已有甲乙方标签"而跳过角色标签兜底。
var partyLabelColonRe = regexp.MustCompile(`[甲乙]\s*方\s*(?:[（(][^）)\n]{0,12}[）)])?\s*[:：]`)

// hasPartyLabel 文本里是否出现明确的甲/乙方标签（有则不启用角色标签兜底）。
func hasPartyLabel(text string) bool {
	return partyLabelColonRe.MatchString(text)
}

// safeGroup 安全取正则子匹配的捕获组（越界返回空串）。
func safeGroup(m []string, i int) string {
	if i < 0 || i >= len(m) {
		return ""
	}
	return m[i]
}

// anchoredParty 取标签锚定的公司名。
//
// 第一分支（「甲方：X」）直接取捕获组；第二分支（「X（甲方）」）要收口：
// RE2 没有负向断言，`([^\s，,；;。\n]{2,60})` 会把叙述性前缀一起吞进来
// （"该合同为星达铜业（甲方）" → 捕获到「该合同为星达铜业」）。
// 因此对第二分支的候选做两步过滤：
//  1. 丢掉含连接词（为/与/和/及/，/；）的候选 —— 公司名不会含这些字；
//  2. 若还有多个候选，取最短的（最贴近标签的那个）。
func anchoredParty(re *regexp.Regexp, text string) string {
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	// 分支 a/b（组1）与分支 c（组2）是「标签：公司名」形态，直接取。
	// 注意：正则带 3 个捕获组，但单次匹配只命中其中一个分支，未命中的组
	// 在 FindAllStringSubmatch 返回里仍是空串（切片长度恒为 4），
	// 用 safeGroup 取值避免任何索引假设。
	for _, m := range matches {
		for _, gi := range []int{1, 2} {
			if v := strings.TrimSpace(strings.Trim(safeGroup(m, gi), " \t，,；;。.、")); v != "" {
				return v
			}
		}
	}
	// 分支 d（组3）是「公司名（甲方）」形态。捕获可能把叙述性前缀一起吞进来
	// （"该合同为星达铜业（甲方）" → 捕获「该合同为星达铜业」），
	// 因此按连接词（为/与/和/及）切分后取**最后一段**——公司名不会含这些字，
	// 而叙述前缀一定以它们结尾。
	best := ""
	for _, m := range matches {
		cand := strings.TrimSpace(strings.Trim(safeGroup(m, 3), " \t，,；;。.、"))
		if cand == "" {
			continue
		}
		if seg := tailAfterConnector(cand); seg != "" {
			cand = seg
		}
		if cand == "出租方" || cand == "承租方" || cand == "甲方" || cand == "乙方" {
			continue
		}
		if best == "" || len([]rune(cand)) < len([]rune(best)) {
			best = cand
		}
	}
	return best
}

// tailAfterConnector 取连接词（为/与/和/及）之后的最后一段。
// 无连接词时原样返回。用于把「该合同为星达铜业」收敛成「星达铜业」。
//
// 必须按 rune 切分：strings.LastIndexAny 返回**字节**下标，中文占 3 字节，
// 用 idx+1 切会切进 UTF-8 字符中间，产出带乱码前缀的字符串
// （真机调试就踩过：A=[\xef\xbf\xbd星达铜业] 断言永远不相等）。
func tailAfterConnector(s string) string {
	rs := []rune(s)
	for i := len(rs) - 1; i >= 0; i-- {
		if strings.ContainsRune("为与和及", rs[i]) {
			if i == len(rs)-1 {
				return s
			}
			return strings.TrimSpace(string(rs[i+1:]))
		}
	}
	return s
}

// ReconcileContractParties 用原文/摘要里的角色标签核对并纠正甲乙方。
//
// 规则（按优先级，任一环拿到值就停）：
//  1. 原文有「甲方：X」→ PartyA = X；有「乙方：Y」→ PartyB = Y；
//  2. 原文没有甲乙方标签、但**成对**写出「出租方：X / 承租方：Y」→ 按角色约定映射
//     （单边角色标签不启用，见下方注释）；
//  3. 原文完全没有角色标签时，用知识库摘要 description 里的
//     「A（甲方）… B（乙方）」关系兜底；
//  4. 全都拿不到 → 原样保留模型结果（不臆造）。
//
// 只覆盖对应侧：原文只写了甲方就只改 PartyA，另一方保持模型值。
func ReconcileContractParties(text, description string, items []ContractExtractionItem) {
	if len(items) == 0 {
		return
	}
	body := strings.TrimSpace(text)
	desc := strings.TrimSpace(description)

	// 原文优先，摘要兜底：两个来源各自的锚点值
	partyA := anchoredParty(partyARe, body)
	partyB := anchoredParty(partyBRe, body)
	if !hasPartyLabel(body) {
		// 无甲乙方标签 → 角色标签。但**必须成对出现**才启用：
		// 真机取证（2026-10-22）补充合同原文只写「承租单位名称：重庆持睿」，
		// 主合同是「甲方：星达 乙方：持恒」——承租方根本不是主合同任何一方。
		// 单边角色标签只有半份证据，用它覆盖完整提取会把主体改错，
		// 因此只在出租方与承租方**同时**明确写出时才按角色约定映射。
		lessor := anchoredParty(lessorRe, body)
		lessee := anchoredParty(lesseeRe, body)
		if lessor != "" && lessee != "" {
			partyA = lessor
			partyB = lessee
		}
	}
	if partyA == "" && partyB == "" && desc != "" {
		// 摘要兜底：只在原文完全没有任何角色标签、且摘要里甲乙方**成对**时启用。
		// 摘要常写「A与B签订…」，那不是甲乙方标签，不能拿来覆盖。
		descA := anchoredParty(partyARe, desc)
		descB := anchoredParty(partyBRe, desc)
		if !hasPartyLabel(body) && !lessorRe.MatchString(body) && !lesseeRe.MatchString(body) &&
			descA != "" && descB != "" {
			partyA = descA
			partyB = descB
		}
	}

	if partyA == "" && partyB == "" {
		return
	}
	for i := range items {
		if partyA != "" {
			items[i].PartyA = partyA
		}
		if partyB != "" {
			items[i].PartyB = partyB
		}
	}
}
