package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// FleetDocumentExtractionResult 车队证照/维保文件解析提取结果。
// Fields 为提取出的字段名→值（小项自动同步到档案分类配置）。
type FleetDocumentExtractionResult struct {
	Kind      string         `json:"kind"`       // fleet_vehicle_document / fleet_driver_document / fleet_maintain_document
	DocType   string         `json:"doc_type"`   // 证照/文件类型（大项）
	VehicleNo string         `json:"vehicle_no"` // 车牌号（如有）
	Fields    map[string]any `json:"fields"`
	Error     string         `json:"error,omitempty"`
}

const fleetVehicleDocSystemPrompt = `你是一个车辆档案信息提取助手。从提供的文档内容中提取车辆相关证照的关键字段。
证照类型可能是：车辆登记证书、行驶证、营运证、道路运输证、保险单、驾驶证、从业资格证等。
输出严格 JSON（不要输出任何其他文字）：
{
  "kind": "fleet_vehicle_document",
  "doc_type": "证照类型（如：行驶证、车辆登记证书、保险单）",
  "vehicle_no": "车牌号（如没有则空字符串）",
  "fields": {
    "证件编号": "…", "所有人": "…", "品牌型号": "…", "车辆识别代号": "…",
    "发动机号": "…", "使用性质": "…", "注册日期": "…", "发证日期": "…",
    "检验有效期": "…", "发证机关": "…", "准驾车型": "…", "驾驶证号": "…",
    "保险公司": "…", "险种": "…", "保单号": "…", "起保日期": "…", "到期日期": "…",
    "保费": "…", "从业资格类别": "…", "有效期限": "…", "核发机关": "…",
    "（其它文档中出现的字段也一并提取）": "…"
  }
}
注意：只提取文档中真实出现的信息，未出现的内容留空字符串；数字金额保留原样。`

const fleetDriverDocSystemPrompt = `你是一个驾驶员档案信息提取助手。从提供的文档内容中提取驾驶员相关证照的关键字段。
证照类型可能是：驾驶证、从业资格证等。
输出严格 JSON（不要输出任何其他文字）：
{
  "kind": "fleet_driver_document",
  "doc_type": "证照类型（如：驾驶证、从业资格证）",
  "vehicle_no": "",
  "fields": {
    "姓名": "…", "性别": "…", "证件类型": "…", "证件号": "…",
    "准驾车型": "…", "从业资格类别": "…", "初次领证日期": "…",
    "有效期限起": "…", "有效期限止": "…", "审验有效期": "…",
    "发证机关": "…", "核发机关": "…", "从业单位": "…",
    "（其它文档中出现的字段也一并提取）": "…"
  }
}
注意：只提取文档中真实出现的信息，未出现的内容留空字符串。`

const fleetMaintainDocSystemPrompt = `你是一个车辆维修保养档案信息提取助手。从提供的文档内容中提取维修保养相关的关键字段。
文件类型可能是：维修合同、维修工单、二级维护记录、保险单等。
输出严格 JSON（不要输出任何其他文字）：
{
  "kind": "fleet_maintain_document",
  "doc_type": "文件类型（如：维修工单、维修合同、二级维护、保险单）",
  "vehicle_no": "车牌号（如没有则空字符串）",
  "fields": {
    "维修厂": "…", "维修项目": "…", "工单号": "…", "维修日期": "…",
    "维修类别": "…", "二级维护记录": "…", "工时费": "…", "材料费": "…",
    "总费用": "…", "合同金额": "…", "合同期限": "…", "保险公司": "…",
    "险种": "…", "保单号": "…", "起保日期": "…", "到期日期": "…", "保费": "…",
    "维修人员": "…", "（其它文档中出现的字段也一并提取）": "…"
  }
}
注意：只提取文档中真实出现的信息，未出现的内容留空字符串；金额保留原样。`

// fleetBuiltinFields 内置证照/文件字段定义（与前端 BUILTIN_CERTS 保持一致），
// 用于该类型尚无用户配置（category subs 为空）时约束提取字段。
var fleetBuiltinFields = map[string]map[string][]string{
	types.FleetCategoryScopeVehicle: {
		"车辆登记证书":        {"证书编号", "车牌号", "VIN码", "发证机关", "发证日期", "证书状态", "存放位置", "是否随车", "备注"},
		"行驶证":          {"编号", "车牌号", "VIN码/车架号", "发动机号", "品牌型号", "车辆类型", "使用性质", "注册日期", "发证日期", "发证机关", "行驶证编号", "有效期", "状态", "备注"},
		"道路运输经营许可证":    {"许可证号", "业户名称", "经营地址", "经营范围", "发证机关", "发证日期", "有效期起", "有效期止", "证件状态", "备注"},
		"道路运输证":        {"道路运输证号", "车牌号", "经营许可证号", "车辆类型", "吨（座）位", "业户名称", "经营地址", "经营范围", "车辆尺寸", "发证日期", "有效期止", "发证机关", "上次审验日期", "下次审验日期", "审验状态", "技术评定等级", "备注"},
		"保险单":          {"保单号", "保险公司", "保险类型", "车辆ID", "车牌号", "VIN码", "被保险人名称", "险种名称", "保额", "保费", "总保费", "起保日期", "终保日期", "保单状态", "缴费状态", "发票号", "到期提醒天数", "是否续保", "备注"},
	},
	types.FleetCategoryScopeDriver: {
		"驾驶证":  {"驾驶证号", "司机姓名", "准驾车型", "初次领证日期", "有效期起", "有效期止", "发证机关", "驾驶证状态", "到期提醒天数", "提醒状态", "备注"},
		"从业资格证": {"从业资格证号", "司机姓名", "从业资格类别", "准运范围", "发证机关", "发证日期", "有效期起", "有效期止", "证件状态", "到期提醒天数", "提醒状态", "审验状态", "备注"},
	},
	types.FleetCategoryScopeMaintain: {
		"维修工单":   {"工单号", "车牌号", "维修厂", "维修项目", "维修日期", "维修类别", "工时费", "材料费", "总费用", "维修人员", "备注"},
		"二级维护":   {"维护记录号", "车牌号", "维护厂", "维护项目", "维护日期", "维护类别", "里程数", "费用", "维护人员", "备注"},
		"保险单":    {"保单号", "保险公司", "保险类型", "车牌号", "被保险人名称", "险种名称", "保额", "保费", "起保日期", "终保日期", "保单状态", "备注"},
	},
}

// FleetBuiltinFieldNames 返回内置字段定义；未定义时返回 nil。
func FleetBuiltinFieldNames(scope, certType string) []string {
	fields, ok := fleetBuiltinFields[scope]
	if !ok {
		return nil
	}
	return fields[certType]
}

func fleetDocSystemPrompt(scope, certType string, fieldNames []string) string {
	certType = strings.TrimSpace(certType)
	// 未指定证照类型 → 旧版自由识别提示
	if certType == "" {
		switch scope {
		case types.FleetCategoryScopeDriver:
			return fleetDriverDocSystemPrompt
		case types.FleetCategoryScopeMaintain:
			return fleetMaintainDocSystemPrompt
		default:
			return fleetVehicleDocSystemPrompt
		}
	}
	kind := "fleet_vehicle_document"
	switch scope {
	case types.FleetCategoryScopeDriver:
		kind = "fleet_driver_document"
	case types.FleetCategoryScopeMaintain:
		kind = "fleet_maintain_document"
	}
	var sb strings.Builder
	sb.WriteString("你是一个档案信息提取助手。本文件证照类型已确定为「")
	sb.WriteString(certType)
	sb.WriteString("」。\n")
	sb.WriteString("输出严格 JSON（不要输出任何其他文字）：\n{\n  \"kind\": \"")
	sb.WriteString(kind)
	sb.WriteString("\",\n  \"doc_type\": \"")
	sb.WriteString(certType)
	sb.WriteString("\",\n  \"vehicle_no\": \"车牌号（如没有则空字符串）\",\n  \"fields\": {\n")
	if len(fieldNames) > 0 {
		lines := make([]string, 0, len(fieldNames))
		for _, f := range fieldNames {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			lines = append(lines, "    \""+f+"\": \"…\"")
		}
		sb.WriteString(strings.Join(lines, ",\n"))
		sb.WriteString("\n  }\n}\n")
	} else {
		sb.WriteString("    \"（按文档实际出现的字段提取）\": \"…\"\n  }\n}\n")
	}
	sb.WriteString("注意：doc_type 必须严格等于「")
	sb.WriteString(certType)
	sb.WriteString("」，严禁修改为其它类型。")
	if len(fieldNames) > 0 {
		sb.WriteString("只提取上面 fields 中列出的字段：文档中真实出现的填值，未出现的内容留空字符串，不要输出未列出的额外字段。")
		sb.WriteString("字段归并提示：文档摘要或正文中出现的「地址」「公司地址」「单位地址」「注册地址」等信息归入「经营地址」字段；「证件状态」「证书状态」等状态字段按有效期自动判断（当前日期之前=已过期，一年内到期=即将到期，否则=有效），能判断就填，不要留空。")
	} else {
		sb.WriteString("只提取文档中真实出现的信息，未出现的内容留空字符串。")
	}
	return sb.String()
}

// normalizeExtractionFields 兜底兼容模型把 JSON Schema 当模板输出的情况：
// fields 内直接嵌 properties/required/type 包装时，从 properties 取真实字段值。
func normalizeExtractionFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return fields
	}
	if _, hasProps := fields["properties"]; hasProps {
		if props, ok := fields["properties"].(map[string]any); ok && len(props) > 0 {
			return props
		}
	}
	return fields
}

// tryParseFleetDocJSON 尝试解析模型输出。
func tryParseFleetDocJSON(raw string, parsed *FleetDocumentExtractionResult) bool {
	cleaned := strings.NewReplacer("```json", "", "```", "", "`", "'").Replace(raw)
	if err := common.ParseLLMJsonResponse(cleaned, parsed); err != nil {
		return false
	}
	if parsed.Kind == "" {
		return false
	}
	if parsed.Fields == nil {
		parsed.Fields = map[string]any{}
	}
	parsed.Fields = normalizeExtractionFields(parsed.Fields)
	return true
}

// ExtractFleetDocumentFromContent calls the configured chat model and parses
// its strict-JSON reply. 与电费/光伏提取一致的"流式优先、多次重试"策略。
// certType 非空时固定输出该证照类型；fieldNames 非空时按字段清单提取；
// systemPrompt 非空时使用用户可配置的提取规则 Prompt（否则回退内置拼装）。
func ExtractFleetDocumentFromContent(ctx context.Context, model chat.Chat, content, scope, certType string, fieldNames []string, systemPrompt string) (*FleetDocumentExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &FleetDocumentExtractionResult{Kind: "fleet_empty"}, nil
	}
	if systemPrompt == "" {
		systemPrompt = fleetDocSystemPrompt(scope, certType, fieldNames)
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	llmCtx := types.WithLLMCallMetadata(ctx, "fleet_document_extract", scope)
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	mkOpts := func() *chat.ChatOptions {
		return &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	}
	var lastErr error
	attempts := []struct {
		label string
		run   func() (string, error)
	}{
		{"nonstream-jsonobject", func() (string, error) {
			// JSON 约束优先：response_format=json_object 从第一重开始，
			// 极大减少"模型输出非 JSON / 自造字段名"的情况
			opts := mkOpts()
			opts.Format = json.RawMessage(`{"type":"json_object"}`)
			resp, err := model.Chat(llmCtx, messages, opts)
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		}},
		{"stream-free", func() (string, error) {
			return chatStreamCollect(llmCtx, model, messages, mkOpts())
		}},
		{"nonstream-free", func() (string, error) {
			resp, err := model.Chat(llmCtx, messages, mkOpts())
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		}},
		{"stream-free-2", func() (string, error) {
			return chatStreamCollect(llmCtx, model, messages, mkOpts())
		}},
	}
	for i, a := range attempts {
		raw, err := a.run()
		if err != nil {
			lastErr = err
			logger.Warnf(ctx, "fleet document extraction attempt %d (%s) error: %v", i+1, a.label, err)
			continue
		}
		var parsed FleetDocumentExtractionResult
		if tryParseFleetDocJSON(raw, &parsed) {
			return &parsed, nil
		}
		lastErr = errParseLLMOutput
	}
	if lastErr == nil {
		lastErr = errParseLLMOutput
	}
	return nil, lastErr
}

var errParseLLMOutput = &parseLLMOutputError{}

type parseLLMOutputError struct{}

func (*parseLLMOutputError) Error() string { return "模型输出解析失败（非合法 JSON）" }
