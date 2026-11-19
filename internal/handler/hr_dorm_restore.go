// 第 64 轮：住宿记录「恢复住宿」（退宿可逆）。
//
// 办理退宿只把住宿状态置为已退宿并写退宿日期/原因，记录本身保留 —— 因此恢复
// 本质上是「把这条住宿记录重新放回房源」，必须重新校验容量与性别，而不是简单
// 把状态改回在住（否则会把已经住满的房间算成超员）。
package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// hrDormBedOptions 房源可用床位号（按床位数生成，1..N）。
func hrDormBedOptions(total int) []string {
	if total <= 0 {
		total = 8
	}
	if total > 8 {
		total = 8
	}
	out := make([]string, 0, total)
	for i := 1; i <= total; i++ {
		out = append(out, fmt.Sprintf("%d号床", i))
	}
	return out
}

// hrDormFindRoom 按「宿舍编号 / 房屋地址 / code」定位房源记录。
func (h *HRHandler) hrDormFindRoom(ctx context.Context, tid int64, key string) (HRDormRecord, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return HRDormRecord{}, false
	}
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module IN ?", tid, []string{hrDormModuleRoom, hrDormModulePublic}).
		Find(&rows).Error; err != nil {
		return HRDormRecord{}, false
	}
	for _, r := range rows {
		if strings.TrimSpace(r.Code) == key ||
			hrDormStr(r.CustomData, "宿舍编号") == key ||
			hrDormStr(r.CustomData, "房屋地址") == key {
			return r, true
		}
	}
	return HRDormRecord{}, false
}

// hrDormRoomStays 同一房源下、指定状态的住宿记录。
func (h *HRHandler) hrDormRoomStays(ctx context.Context, tid int64, key string, onlyLiving bool) []HRDormRecord {
	var rows []HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND module = ?", tid, hrDormModuleStay).Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]HRDormRecord, 0, len(rows))
	for _, r := range rows {
		code := hrDormStr(r.CustomData, "宿舍编号/房屋地址")
		if code == "" {
			code = r.Code
		}
		if code != key {
			continue
		}
		status := hrDormStr(r.CustomData, "住宿状态")
		if status == "" {
			status = r.Status
		}
		if onlyLiving && status == "已退宿" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// hrRestoreStay 恢复单条住宿记录；返回重新分配到的床位号。
func (h *HRHandler) hrRestoreStay(ctx context.Context, tid int64, id string) (string, error) {
	var row HRDormRecord
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ? AND module = ?", tid, id, hrDormModuleStay).
		First(&row).Error; err != nil {
		return "", fmt.Errorf("住宿记录不存在")
	}
	d := row.CustomData
	if d == nil {
		d = map[string]any{}
	}
	status := hrDormStr(d, "住宿状态")
	if status == "" {
		status = row.Status
	}
	if status != "已退宿" {
		return "", fmt.Errorf("该记录当前为「%s」，无需恢复住宿", status)
	}
	roomKey := hrDormStr(d, "宿舍编号/房屋地址")
	if roomKey == "" {
		roomKey = row.Code
	}
	if strings.TrimSpace(roomKey) == "" {
		return "", fmt.Errorf("住宿记录缺少宿舍编号/房屋地址，无法恢复")
	}
	// 第 65 轮：恢复住宿同样受入住准入约束（满员 / 维修中 拒绝，排除自身占位）
	if err := h.hrDormAdmitCheck(ctx, tid, roomKey, row.ID); err != nil {
		return "", err
	}
	room, ok := h.hrDormFindRoom(ctx, tid, roomKey)
	if !ok {
		return "", fmt.Errorf("找不到房源「%s」，请先在宿舍档案中补建", roomKey)
	}
	total := hrDormInt(room.CustomData, "床位数")
	living := h.hrDormRoomStays(ctx, tid, roomKey, true)
	if total > 0 && len(living) >= total {
		return "", fmt.Errorf("房源「%s」已住满（%d/%d），请先调宿或扩容后再恢复", roomKey, len(living), total)
	}
	// 性别校验：夫妻房不限；房源「性别限制」为男/女时按花名册性别比对
	limit := hrDormStr(room.CustomData, "性别限制")
	roomType := hrDormStr(room.CustomData, "房型")
	if limit == "男" || limit == "女" {
		if roomType != "夫妻房" {
			name := hrDormStr(d, "员工姓名")
			var gender string
			if emp, ok := h.hrDormEmployeeIndex(ctx, tid)[strings.TrimSpace(name)]; ok {
				gender = strings.TrimSpace(emp.Gender)
			}
			if gender != "" && gender != "不限" && gender != limit {
				return "", fmt.Errorf("房源「%s」仅限%s性，员工性别为%s，无法恢复", roomKey, limit, gender)
			}
		}
	}
	// 床位：原床位若已被在住记录占用，自动分配下一个空闲床位
	bed := strings.TrimSpace(hrDormStr(d, "床位号"))
	occupied := map[string]bool{}
	for _, r := range living {
		if b := strings.TrimSpace(hrDormStr(r.CustomData, "床位号")); b != "" {
			occupied[b] = true
		}
	}
	if bed == "" || occupied[bed] {
		bed = ""
		for _, cand := range hrDormBedOptions(total) {
			if !occupied[cand] {
				bed = cand
				break
			}
		}
		if bed == "" {
			return "", fmt.Errorf("房源「%s」已无空闲床位，无法恢复", roomKey)
		}
	}
	d["床位号"] = bed
	d["退宿日期"] = ""
	d["退宿原因"] = ""
	d["住宿状态"] = "在住"
	row.CustomData = d
	row.Status = "在住"
	if err := h.db.WithContext(ctx).Save(&row).Error; err != nil {
		return "", fmt.Errorf("恢复住宿失败: %w", err)
	}
	// 房源派生状态落库（列表仍按实时口径重算，这里只是让导出/打印也一致）
	h.hrDormSyncRoomState(ctx, tid, room, roomKey)
	return bed, nil
}

// hrDormSyncRoomState 把「可用床位数 / 宿舍状态」写回房源记录。
func (h *HRHandler) hrDormSyncRoomState(ctx context.Context, tid int64, room HRDormRecord, roomKey string) {
	total := hrDormInt(room.CustomData, "床位数")
	used := len(h.hrDormRoomStays(ctx, tid, roomKey, true))
	if room.CustomData == nil {
		room.CustomData = map[string]any{}
	}
	status, free := hrDormRoomStatus(total, used, hrDormStr(room.CustomData, "宿舍状态"))
	room.CustomData["可用床位数"] = free
	room.CustomData["宿舍状态"] = status
	_ = h.db.WithContext(ctx).Save(&room).Error
}

// RestoreDormStay POST /hr/dorm/stays/:id/restore
func (h *HRHandler) RestoreDormStay(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	bed, err := h.hrRestoreStay(ctx, tid, c.Param("id"))
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	hrOK(c, gin.H{"restored": 1, "bed": bed})
}

// RestoreDormStays POST /hr/dorm/stays/restore {ids: []string}
//
// 批量按条独立处理：一条失败不影响其它记录，失败原因按记录回传给前端提示。
func (h *HRHandler) RestoreDormStays(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		hrFail(c, "请选择要恢复住宿的记录")
		return
	}
	restored := 0
	failed := make([]map[string]string, 0)
	for _, id := range req.IDs {
		if _, err := h.hrRestoreStay(ctx, tid, id); err != nil {
			failed = append(failed, map[string]string{"id": id, "message": err.Error()})
			continue
		}
		restored++
	}
	if restored == 0 && len(failed) > 0 {
		hrFail(c, failed[0]["message"])
		return
	}
	hrOK(c, gin.H{"restored": restored, "failed": failed})
}
