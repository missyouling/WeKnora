// 发票字段补齐与交叉推算（第 24 轮任务二）。
//
// 背景（VM 真机抽样）：通行费汇总单里大量记录出现三类缺项 ——
//
//	缺项 A「车牌号」整列为 `-`：车牌既可能出现在备注（`车牌号：渝A75R1C`），
//	          更常直接出现在明细行（`*生产生活服务*通行费 渝A75R1C 客车 20261022`），
//	          没有 `车牌号：` 前缀。旧路径只认前缀形态，且顶层 `Fields["车牌号"]`
//	          在部分规则里没写，前端列读不到。
//	缺项 B「开票人」多数为空：多页 PDF 切块时，位于每页底部的 `开票人：X`
//	          容易落到下一个 chunk 或被切丢；且票面常渲染成 `开 票 人 ：\n X`。
//	缺项 C 金额/税率/税额/价税合计全 0：段落只有表头、数据行在下一 chunk
//	          （chunk 边界把「号码行」与「数据行」切开），旧规则按段配对时整段落空。
//
// 本文件只做**确定性补齐**，不调用模型、不改 docreader：
//   - normalizeInvoiceExtras 在 Normalize 收尾处跑一次，对每张票做
//     车牌/开票人补齐 + 金额四段交叉推算 + 字段名归一；
//   - 所有补齐都要求「有原文依据」：算不出来的宁可留空，绝不填 0 或猜。
package service

import (
	"regexp"
	"strings"
)

// plateTopFieldKey 是顶层车牌字段在 `Fields` 桶里的键名。
//
// 用中文「车牌号」而不是 plate_no：分类字段配置（subs）里该字段名就是中文，
// 前端动态列按 subs.name 读取；同时 plate_no 作为别名一起写，供明细行/历史数据
// 的读法兜底（前端统一按 plate_no || license_plate || vehicle_no 读）。
const plateTopFieldKey = "车牌号"

var (
	// cnPlateRe 标准中国车牌（含新能源 6 位、挂/学/警/港/澳/领/使/电 后缀）。
	// 首字是省份简称，次字是发牌机关字母（I/O 不用），后 5~6 位字母数字。
	cnPlateRe = regexp.MustCompile(
		"[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼]" +
			"[A-HJ-NP-Z][A-HJ-NP-Z0-9]{4,5}[A-HJ-NP-Z0-9挂学警港澳领使电]?")

	// plateLabelRe 带标签形态：`车牌号：渝A75R1C` / `车牌号码: 渝D95256`。
	plateLabelRe = regexp.MustCompile(
		"车\\s*牌\\s*号?\\s*码?\\s*[:：]?\\s*([京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-HJ-NP-Z][A-HJ-NP-Z0-9]{4,5}[A-HJ-NP-Z0-9挂学警港澳领使电]?)")

	// issuerLooseRe 开票人宽松匹配：允许字间空格、允许冒号后换行。
	//
	// 票面常渲染成 `开 票 人 ：\n 杨希`（PDF 文本层按字断行），旧正则
	// `开票人\s*[:：]\s*(\S+)` 在「开 票 人」上直接失配 → 开票人整列空白。
	issuerLooseRe = regexp.MustCompile(`开\s*票\s*人\s*[:：]?\s*\n?\s*([\p{Han}·]{2,8})`)

	// issuerNoise 非人名噪词：模板占位与系统账号，命中即视为未提取。
	issuerNoise = map[string]struct{}{
		"管理员": {}, "系统": {}, "电子发票": {}, "备注": {}, "开票": {}, "复核": {},
		"收款人": {}, "销售方": {}, "购买方": {}, "国家税务总局": {}, "发票专用章": {},
	}
)

// normalizeInvoiceExtras 对全部发票做确定性字段补齐（原地修改）。
//
// 幂等：已有值一律不覆盖（补齐只填空）。返回实际补齐的字段数（供日志）。
func normalizeInvoiceExtras(res *InvoiceExtractionResult) int {
	if res == nil || len(res.Invoices) == 0 {
		return 0
	}
	fixed := 0
	for i := range res.Invoices {
		fixed += completeInvoiceExtras(&res.Invoices[i])
	}
	return fixed
}

// completeInvoiceExtras 补齐单张发票的车牌 / 开票人 / 金额四段。
func completeInvoiceExtras(inv *InvoiceExtractionItem) int {
	if inv == nil {
		return 0
	}
	fixed := 0
	fixed += fillInvoicePlate(inv)
	fixed += fillInvoiceIssuer(inv)
	fixed += reconcileInvoiceMoney(inv)
	return fixed
}

// fillInvoicePlate 补齐车牌号（三级优先级，全部有原文依据）。
//
//	① 顶层 `Fields["车牌号"]` 已有 → 只做格式清洗与别名同步；
//	② 备注里的 `车牌号：X`；
//	③ 全文扫描（明细行 items 的 name/extra、remark 里裸车牌）。
//
// 为什么必须三级：真机里同一批票三种形态都存在 —— 有的只写备注，
// 有的只在明细行 `*通行费 渝D95256 货车 20261022`（无 `车牌号：` 前缀），
// 有的两者都有。只认一种就整列空白。
func fillInvoicePlate(inv *InvoiceExtractionItem) int {
	if inv == nil {
		return 0
	}
	plate := plateOfFields(inv.Fields)
	if plate == "" {
		if m := plateLabelRe.FindStringSubmatch(inv.Remark); len(m) > 1 {
			plate = m[1]
		}
	}
	if plate == "" {
		plate = plateFromItems(inv.Items)
	}
	if plate == "" {
		// 最后一道：明细行/备注里任何裸车牌（如 `*经营租赁*通行费 渝A75R1C 客车`）
		for _, it := range inv.Items {
			if m := cnPlateRe.FindString(it.Name); m != "" {
				plate = m
				break
			}
		}
		if plate == "" {
			if m := cnPlateRe.FindString(inv.Remark); m != "" {
				plate = m
			}
		}
	}
	if plate == "" {
		return 0
	}
	fixed := 0
	if inv.Fields == nil {
		inv.Fields = map[string]any{}
	}
	// 中文键是权威（分类 subs 用中文名），plate_no 是读法别名
	if cur, _ := inv.Fields[plateTopFieldKey].(string); strings.TrimSpace(cur) != plate {
		inv.Fields[plateTopFieldKey] = plate
		fixed++
	}
	if cur, _ := inv.Fields["plate_no"].(string); strings.TrimSpace(cur) != plate {
		inv.Fields["plate_no"] = plate
		fixed++
	}
	// 明细行同步（前端列表对分类子列有 items[0].extra 回退路径）
	for i := range inv.Items {
		if inv.Items[i].Extra == nil {
			inv.Items[i].Extra = map[string]any{}
		}
		if cur, _ := inv.Items[i].Extra["plate_no"].(string); strings.TrimSpace(cur) != plate {
			inv.Items[i].Extra["plate_no"] = plate
			fixed++
		}
	}
	return fixed
}

// plateOfFields 从 Fields 桶取车牌（兼容中文键与三种英文别名）。
func plateOfFields(fields map[string]any) string {
	if len(fields) == 0 {
		return ""
	}
	for _, k := range []string{plateTopFieldKey, "plate_no", "license_plate", "vehicle_no"} {
		if v, ok := fields[k].(string); ok {
			if p := normalizePlate(v); p != "" {
				return p
			}
		}
	}
	return ""
}

// plateFromItems 从明细行的 extra / name 取车牌。
func plateFromItems(items []InvoiceExtractionLine) string {
	for _, it := range items {
		for _, k := range []string{"plate_no", "license_plate", "vehicle_no"} {
			if v, ok := it.Extra[k].(string); ok {
				if p := normalizePlate(v); p != "" {
					return p
				}
			}
		}
		if m := plateLabelRe.FindStringSubmatch(it.Name); len(m) > 1 {
			return m[1]
		}
		if m := cnPlateRe.FindString(it.Name); m != "" {
			return m
		}
	}
	return ""
}

// normalizePlate 把候选串收敛成标准车牌；不合法返回空。
//
// 模型/OCR 常把车牌连同前后文一起返回（`车牌号渝A75R1C`、`渝A75R1C客车`），
// 这里用标准正则从串里抠出第一段合法车牌。
func normalizePlate(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if cnPlateRe.MatchString(s) {
		return cnPlateRe.FindString(s)
	}
	return ""
}

// fillInvoiceIssuer 补齐开票人（宽松正则 + 噪词过滤）。
func fillInvoiceIssuer(inv *InvoiceExtractionItem) int {
	if inv == nil {
		return 0
	}
	if name := cleanIssuerName(inv.Issuer); name != "" {
		if name != inv.Issuer {
			inv.Issuer = name
			return 1
		}
		return 0
	}
	// 顶层为空 → 从备注/明细行文本里找（多页切块时开票人常落到备注）
	for _, src := range []string{inv.Remark, itemsTextOf(inv.Items)} {
		if m := issuerLooseRe.FindStringSubmatch(src); len(m) > 1 {
			if name := cleanIssuerName(m[1]); name != "" {
				inv.Issuer = name
				return 1
			}
		}
	}
	if inv.Issuer != "" {
		// 原值是噪词（如「管理员」）→ 清空，避免列表展示假人名
		inv.Issuer = ""
		return 1
	}
	return 0
}

// cleanIssuerName 校验并清洗开票人：去空白、剔噪词、长度 2~8 个汉字。
func cleanIssuerName(raw string) string {
	s := strings.TrimSpace(strings.ReplaceAll(raw, " ", ""))
	if s == "" {
		return ""
	}
	if _, noise := issuerNoise[s]; noise {
		return ""
	}
	r := []rune(s)
	if len(r) < 2 || len(r) > 8 {
		return ""
	}
	// 必须全是汉字或间隔点（排除数字/字母/符号混入）
	for _, c := range r {
		if c == '·' {
			continue
		}
		if c < 0x4e00 || c > 0x9fff {
			return ""
		}
	}
	return s
}

// itemsTextOf 把明细行的可读文本拼起来（供开票人/车牌兜底扫描）。
func itemsTextOf(items []InvoiceExtractionLine) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Name)
		b.WriteString(" ")
	}
	return b.String()
}

// reconcileInvoiceMoney 交叉推算金额四段（金额 / 税额 / 税率 / 价税合计）。
//
// 推算规则（只在「缺」的方向上补，已有值一律不覆盖）：
//
//	有 金额 + 税率          → 税额 = 金额 × 税率
//	有 金额 + 税额          → 税率 = 税额 ÷ 金额（保留两位百分数精度）
//	有 金额 + 税额          → 价税合计 = 金额 + 税额
//	有 价税合计 + 税额      → 金额 = 价税合计 − 税额
//	有 价税合计 + 金额      → 税额 = 价税合计 − 金额
//	有 价税合计 + 税率      → 金额 = 价税合计 ÷ (1+税率)，税额 = 价税合计 − 金额
//
// 红线：**至少要有两项**才推算。只有一项时任何推算都是猜（把 0 填成 0 更糟），
// 宁可留空让用户在抽屉里补。这是「消灭 0.00 空行」与「不伪造数据」的分界。
func reconcileInvoiceMoney(inv *InvoiceExtractionItem) int {
	if inv == nil {
		return 0
	}
	fixed := 0
	amt, tax, total, rate := inv.Amount, inv.Tax, inv.TotalAmount, inv.TaxRate
	hasAmt := amt != nil && *amt != 0
	hasTax := tax != nil && *tax != 0
	hasTotal := total != nil && *total != 0
	hasRate := rate != nil && *rate != 0

	switch {
	case hasAmt && hasRate && !hasTax:
		v := roundMoney(*amt * *rate)
		inv.Tax = &v
		tax, hasTax = &v, true
		fixed++
	case hasAmt && hasTax && !hasRate:
		v := roundMoney(*tax / *amt * 100) / 100
		if v > 0 && v < 1 {
			inv.TaxRate = &v
			rate, hasRate = &v, true
			fixed++
		}
	case hasTotal && hasTax && !hasAmt:
		v := roundMoney(*total - *tax)
		inv.Amount = &v
		amt, hasAmt = &v, true
		fixed++
	case hasTotal && hasAmt && !hasTax:
		v := roundMoney(*total - *amt)
		inv.Tax = &v
		tax, hasTax = &v, true
		fixed++
	case hasTotal && hasRate && !hasAmt && !hasTax:
		base := roundMoney(*total / (1 + *rate))
		t := roundMoney(*total - base)
		inv.Amount, inv.Tax = &base, &t
		amt, tax, hasAmt, hasTax = &base, &t, true, true
		fixed += 2
	}
	// 价税合计：金额与税额都在时补（无论上面是否刚推算出来）
	if !hasTotal && hasAmt && hasTax {
		v := roundMoney(*amt + *tax)
		inv.TotalAmount = &v
		fixed++
	}
	_ = rate
	return fixed
}

// applyPlateAndIssuerFromFullText 用**整份文档原文**给某张票补车牌/开票人。
//
// 这是「chunks 双源融合」的落点：段落内找不到（数据行被切到下一个 chunk）时，
// 按发票号在全文里定位，取该票号之后到下一个票号之前的一小段做兜底扫描。
// 只用于补齐，绝不覆盖已有值；找不到就留空。
func applyPlateAndIssuerFromFullText(inv *InvoiceExtractionItem, fullText string) int {
	if inv == nil || inv.InvoiceNo == "" || fullText == "" {
		return 0
	}
	at := strings.Index(fullText, inv.InvoiceNo)
	if at < 0 {
		return 0
	}
	// 票号之后最多取 1500 字（一页票据的正文量级），避免跨票取到别人的字段
	seg := fullText[at:]
	if len(seg) > 1500 {
		seg = seg[:1500]
	}
	fixed := 0
	if plateOfFields(inv.Fields) == "" {
		if p := plateFromSegment(seg); p != "" {
			if inv.Fields == nil {
				inv.Fields = map[string]any{}
			}
			inv.Fields[plateTopFieldKey] = p
			inv.Fields["plate_no"] = p
			fixed++
		}
	}
	if cleanIssuerName(inv.Issuer) == "" {
		if m := issuerLooseRe.FindStringSubmatch(seg); len(m) > 1 {
			if name := cleanIssuerName(m[1]); name != "" {
				inv.Issuer = name
				fixed++
			}
		}
	}
	return fixed
}

// plateFromSegment 从一段文本里取车牌（标签优先，其次裸车牌）。
func plateFromSegment(seg string) string {
	if m := plateLabelRe.FindStringSubmatch(seg); len(m) > 1 {
		return m[1]
	}
	return cnPlateRe.FindString(seg)
}
