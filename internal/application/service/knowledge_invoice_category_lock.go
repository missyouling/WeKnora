// 发票分类铁律：上传时所选分类是唯一权威来源。
//
// 事故背景（VM 实测 2026-10-19）：同一批上传 5 个通行费汇总单，3 个分类正确、
// 2 个落进「其它票据」。原因是分类写入依赖「parse 完成后前端另发一次 PUT」，
// 而提取流程落库时把 custom_metadata **整列替换**为提取结果 JSON（不含分类键）
// —— 谁写在后谁赢，于是交替丢失。此外 invoice_category 字段从未被写入过。
//
// 本文件提供三件套：
//  1. mergeInvoiceMetaPreservingUserKeys —— 落库合并，用户分类绝不被动覆盖；
//  2. resolveInvoiceCertType —— 分类来源的唯一优先级链；
//  3. LockInvoiceCategory —— 提取结果分类强制锁定。
package service

import (
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// UserOwnedInvoiceMetaKeys 是「由用户/上传流程拥有、提取流程不得凭默认值覆盖」
// 的 metadata 键。提取结果里**不含**这些键，因此合并时一律从库中现值保留；
// 若新值显式携带了同名键（用户主动改分类），则以新值为准。
var UserOwnedInvoiceMetaKeys = []string{
	"invoice_category",
	"fleet_cert_type",
	"fleet_scope",
	"auto_deleted_count",
	"auto_deleted",
	"auto_deleted_reason",
	"auto_deleted_at",
	"pending_file_delete",
	"pending_file_delete_at",
}

// mergeInvoiceMetaPreservingUserKeys 把 fresh（本次提取产物）合并到 existing
// （库中现值）之上，并对 UserOwnedInvoiceMetaKeys 做「新值缺省则保留旧值」。
//
// 语义要点：
//   - fresh 的键覆盖 existing 同名键（提取状态/发票数组用新值）；
//   - 用户拥有的键：fresh 未提供 → 保留 existing；fresh 提供 → 用 fresh
//     （人工改分类必须生效）；
//   - existing 为空/损坏 → 直接返回 fresh（宁可丢分类，也不能让提取失败）。
func mergeInvoiceMetaPreservingUserKeys(existing, fresh []byte) []byte {
	if len(fresh) == 0 {
		return existing
	}
	if len(existing) == 0 {
		return fresh
	}
	var oldMap map[string]interface{}
	if err := json.Unmarshal(existing, &oldMap); err != nil || oldMap == nil {
		return fresh
	}
	var newMap map[string]interface{}
	if err := json.Unmarshal(fresh, &newMap); err != nil || newMap == nil {
		return fresh
	}
	// 新值逐键覆盖：newMap 未出现的键（含用户拥有的分类/删除历史）自动保留旧值。
	for k, v := range newMap {
		oldMap[k] = v
	}
	// 安全阀：提取出了真实发票，说明这份文件活着 —— 清掉可能残留的
	// 「删空待清理」标记，避免巡检把有效文件当空壳清理。
	if inv, ok := newMap["invoices"].([]interface{}); ok && len(inv) > 0 {
		delete(oldMap, "pending_file_delete")
		delete(oldMap, "pending_file_delete_at")
	}
	out, err := json.Marshal(oldMap)
	if err != nil {
		return fresh
	}
	return out
}

// resolveInvoiceCertType 计算本次提取应用的权威分类。
//
// 优先级（高 → 低）：
//  1. requestCert —— 本次请求显式指定的分类（上传弹窗 / 手动重新提取）；
//  2. invoiceCategory —— 上传创建记录时即持久化的权威分类；
//  3. fleetCert —— 历史字段（上一版只写这个，存量数据都在这里）。
//
// 全空返回空串，表示「无权威分类」，此时按模型/规则结果走。
func resolveInvoiceCertType(requestCert, invoiceCategory, fleetCert string) string {
	for _, v := range []string{requestCert, invoiceCategory, fleetCert} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// LockInvoiceCategory 把提取结果的分类强制锁定为 certType。
//
// certType 非空时：每张票的 InvoiceType 与 Category 都被写成 certType，
// 模型返回的任何值（普通发票/专用发票/其它票据/空串）一律无效。
// certType 为空时不做任何改动（保持既有「无权威分类则按模型结果」语义）。
func LockInvoiceCategory(res *InvoiceExtractionResult, certType string) {
	if res == nil {
		return
	}
	if certType = strings.TrimSpace(certType); certType == "" {
		return
	}
	for i := range res.Invoices {
		res.Invoices[i].InvoiceType = certType
		res.Invoices[i].Category = certType
	}
}

// ResolveInvoiceCertType 从 custom_metadata 解析权威分类并应用优先级链。
// 调用方通常只需传请求入参与库中 metadata，无需自己解 JSON。
func ResolveInvoiceCertType(requestCert string, raw types.JSON) string {
	return resolveInvoiceCertType(requestCert, InvoiceCategoryFromMeta(raw), legacyFleetCertType(raw))
}

// legacyFleetCertType 只读历史字段 fleet_cert_type（不含 invoice_category）。
func legacyFleetCertType(raw types.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var m struct {
		FleetCertType string `json:"fleet_cert_type"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m.FleetCertType)
}

// InvoiceCategoryFromMeta 从 custom_metadata 中读取权威分类（invoice_category
// 优先，回退历史字段 fleet_cert_type）。meta 为空/损坏返回空串。
func InvoiceCategoryFromMeta(raw types.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var m struct {
		InvoiceCategory string `json:"invoice_category"`
		FleetCertType   string `json:"fleet_cert_type"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return resolveInvoiceCertType("", m.InvoiceCategory, m.FleetCertType)
}

// SeedInvoiceCategoryMeta 构造「上传即持久化」的分类 metadata。
// scope 为空时只写分类键。
func SeedInvoiceCategoryMeta(category, scope string) types.JSON {
	m := map[string]interface{}{
		"invoice_category": strings.TrimSpace(category),
		"fleet_cert_type":  strings.TrimSpace(category),
	}
	if scope = strings.TrimSpace(scope); scope != "" {
		m["fleet_scope"] = scope
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return types.JSON(raw)
}
