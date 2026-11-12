// 花名册新增/编辑请求的中文键兼容层（第 46 轮）。
//
// 前端详情抽屉按 76 个中文表头提交 `form.data[中文键]`，直接 ShouldBindJSON 到
// HREmployee struct 会把这些键全部丢掉。这里把原始 body 再解一次 map，
// 把中文键无损并入 custom_data，并把已知主列同步映射到 struct 字段。
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

// hrBindRosterPayload 绑定 struct 并额外返回原始 map（body 可重复读）。
func hrBindRosterPayload(c *gin.Context, req *HREmployee) (map[string]any, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err := json.Unmarshal(body, req); err != nil {
		return nil, err
	}
	raw := map[string]any{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return req.CustomData, nil
	}
	return raw, nil
}

// hrRosterStructKeys 是 struct 自身的英文 json 键，不应写进 custom_data。
var hrRosterStructKeys = map[string]bool{
	"id": true, "tenant_id": true, "employee_id": true, "name": true, "department": true,
	"position": true, "gender": true, "hire_date": true, "age": true, "work_years": true,
	"birth_month": true, "education": true, "political_status": true, "work_clothing_size": true,
	"safety_shoe_size": true, "household_type": true, "ethnicity": true, "native_place": true,
	"id_address": true, "id_number": true, "marital_status": true, "social_insurance": true,
	"has_birth": true, "phone": true, "social_insurance_number": true,
	"provident_fund_number": true, "emergency_contact": true, "emergency_phone": true,
	"current_address": true, "graduate_school": true, "major": true, "graduation_time": true,
	"email": true, "remarks": true, "status": true, "resign_date": true, "resign_reasons": true,
	"resign_proof_path": true, "resign_remark": true, "custom_data": true,
	"created_at": true, "updated_at": true,
}

// hrApplyRosterRaw 把原始请求里的中文键并入 custom_data 并同步主列。
func hrApplyRosterRaw(req *HREmployee, raw map[string]any, existing *HREmployee) {
	custom := req.CustomData
	if custom == nil {
		custom = map[string]any{}
	}
	if existing != nil && len(existing.CustomData) > 0 {
		for k, v := range existing.CustomData {
			if _, ok := custom[k]; !ok {
				custom[k] = v
			}
		}
	}
	for k, v := range raw {
		if hrRosterStructKeys[k] || v == nil {
			continue
		}
		std := hrRosterHeaderKey(k)
		if std == "" {
			std = strings.TrimSpace(k)
		}
		// 第 52 轮：保存统一清洗（trim / 去不可见字符 / 号码去空格横杠 / x→X）
		value := hrCleanImportValue(std, fmt.Sprint(v))
		if value != "" && hrIsRosterDateField(std) {
			value = normalizeHRDateValue(value)
		}
		// 新字段名与存量别名双写：花名册、离职管理、劳动合同三种读法都能命中。
		for _, key := range hrFieldPersistKeys(std) {
			custom[key] = value
		}
	}
	// 派生/扩展字段即使不在 76 表头里也要能收下
	for _, k := range append(append([]string{}, hrRosterDerivedFields...), hrRosterExtraResignFields...) {
		if v, ok := raw[k]; ok && v != nil {
			custom[k] = fmt.Sprint(v)
		}
	}
	strOf := func(key string) string {
		if v, ok := custom[key]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
	if v := strOf("工号"); v != "" {
		req.EmployeeID = v
	}
	if v := strOf("姓名"); v != "" {
		req.Name = v
	}
	if v := strOf("部门"); v != "" {
		req.Department = v
	}
	if v := strOf("职务"); v != "" {
		req.Position = v
	}
	if v := strOf("岗位"); v != "" && req.Position == "" {
		req.Position = v
	}
	if v := strOf("性别"); v != "" {
		req.Gender = v
	}
	if v := strOf("入职日期"); v != "" {
		req.HireDate = v
	}
	if v := strOf("离职日期"); v != "" {
		req.ResignDate = v
	}
	if v := strOf("手机号码"); v != "" {
		req.Phone = v
	}
	if v := strOf("证件号码"); v != "" {
		req.IDNumber = normalizeIDNumber(v)
	}
	if v := strOf("个人社保账号"); v != "" {
		req.SocialInsuranceNumber = v
	}
	if v := strOf("个人公积金账号"); v != "" {
		req.ProvidentFundNumber = v
	}
	if v := strOf("紧急联系人姓名"); v != "" {
		req.EmergencyContact = v
	}
	if v := strOf("紧急联系人电话"); v != "" {
		req.EmergencyPhone = v
	}
	if v := strOf("最高学历"); v != "" {
		req.Education = v
	}
	if v := strOf("政治面貌"); v != "" {
		req.PoliticalStatus = v
	}
	if v := strOf("籍贯"); v != "" {
		req.NativePlace = v
	}
	if v := strOf("民族"); v != "" {
		req.Ethnicity = v
	}
	if v := strOf("户口性质"); v != "" {
		req.HouseholdType = v
	}
	if v := strOf("户籍地址"); v != "" {
		req.IDAddress = v
	}
	if v := strOf("个人邮箱"); v != "" {
		req.Email = v
	}
	if v := strOf("离职原因"); v != "" {
		req.ResignReasons = v
	}

	rawStatus := strOf("员工状态")
	if rawStatus == "" {
		rawStatus = strOf("在职状态")
	}
	if rawStatus == "" {
		rawStatus = req.Status
	}
	req.Status = normalizeHREmployeeStatus(rawStatus, strOf("工作性质"))
	custom["员工状态"] = req.Status
	custom["在职状态"] = req.Status

	hrEnrichEmployee(req)
	if req.Age != "" {
		custom["年龄"] = req.Age
	}
	if req.WorkYears != "" {
		custom["工龄"] = req.WorkYears
	}
	req.CustomData = custom
}
