// 第 65 轮：入住准入校验（安排入住 / 办理调宿 / 恢复住宿 全链路）。
//
// 规则：目标房源状态为「满员」或「维修中」，或可用床位数 <= 0 时一律拒绝。
// 状态一律实时派生（维修中 = 房源手填宿舍状态，满员 = 床位数 - 在住人数 <= 0），
// 绝不读列表上的静态缓存值。
package handler

import (
	"context"
	"fmt"
	"strings"
)

// hrDormAdmitCheck 校验能否把人员放进目标房源。
//
// excludeStayID 用于调宿 / 恢复场景：计算占用时必须排除当前这条住宿记录本身，
// 否则「从 301 调到 301」会因为把自己算进去而误判满员。
func (h *HRHandler) hrDormAdmitCheck(ctx context.Context, tid int64, roomKey, excludeStayID string) error {
	roomKey = strings.TrimSpace(roomKey)
	if roomKey == "" {
		return fmt.Errorf("请选择宿舍编号/房屋地址")
	}
	room, ok := h.hrDormFindRoom(ctx, tid, roomKey)
	if !ok {
		return fmt.Errorf("找不到房源「%s」，请先在宿舍档案中补建", roomKey)
	}
	manual := strings.TrimSpace(hrDormStr(room.CustomData, "宿舍状态"))
	if manual == "维修中" {
		return fmt.Errorf("房源「%s」正在维修中，暂不可安排入住", roomKey)
	}
	total := hrDormInt(room.CustomData, "床位数")
	living := h.hrDormRoomStays(ctx, tid, roomKey, true)
	used := 0
	for _, s := range living {
		if excludeStayID != "" && s.ID == excludeStayID {
			continue
		}
		used++
	}
	status, free := hrDormRoomStatus(total, used, manual)
	if status == "满员" || (total > 0 && free <= 0) {
		return fmt.Errorf("房源「%s」已住满（%d/%d），请先调宿或扩容", roomKey, used, total)
	}
	return nil
}
