package api

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
	"ams/internal/service"
)

func (s *Server) listRepairs(c *gin.Context) {
	page, size, offset := pagination(c)
	u := middleware.CurrentUser(c)

	q := s.db.Model(&model.RepairTicket{})
	if !middleware.HasPerm(u, model.PermRepairHandle) || c.Query("mine") == "1" {
		q = q.Where("reporter_id = ?", u.ID)
	}
	if v := multiQuery(c, "status"); len(v) > 0 {
		q = q.Where("status IN ?", v)
	}
	if v := c.Query("assetId"); v != "" {
		q = q.Where("asset_id = ?", v)
	}

	var total int64
	q.Count(&total)

	var tickets []model.RepairTicket
	if err := q.Preload("Asset").Preload("Asset.Category").Preload("Reporter").Preload("Handler").
		Order("CASE status WHEN 'open' THEN 0 WHEN 'processing' THEN 1 ELSE 2 END, created_at DESC").
		Limit(size).Offset(offset).Find(&tickets).Error; err != nil {
		response.Error(c, "查询失败："+err.Error())
		return
	}
	response.OKPage(c, tickets, total, page, size)
}

type createRepairReq struct {
	AssetID uint   `json:"assetId" binding:"required"`
	Fault   string `json:"fault" binding:"required"`
}

// createRepair 提交报修：设备随即转入"维修中"，从可领用池里摘出去。
func (s *Server) createRepair(c *gin.Context) {
	var req createRepairReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请选择设备并描述故障现象")
		return
	}
	var asset model.Asset
	if err := s.db.First(&asset, req.AssetID).Error; err != nil {
		response.NotFound(c, "设备不存在")
		return
	}
	if asset.Status == model.AssetScrapped {
		response.BadRequest(c, "该设备已报废，无需报修")
		return
	}
	var pending int64
	s.db.Model(&model.RepairTicket{}).
		Where("asset_id = ? AND status IN ?", req.AssetID, []string{model.RepairOpen, model.RepairProcessing}).
		Count(&pending)
	if pending > 0 {
		response.BadRequest(c, "该设备已有未完结的报修工单")
		return
	}

	u := middleware.CurrentUser(c)
	now := time.Now()
	var seq int64
	s.db.Model(&model.RepairTicket{}).Where("ticket_no LIKE ?", "RP"+now.Format("20060102")+"%").Count(&seq)
	ticket := model.RepairTicket{
		TicketNo: fmt.Sprintf("RP%s%04d", now.Format("20060102"), seq+1),
		AssetID:  req.AssetID, ReporterID: u.ID, Fault: req.Fault, Status: model.RepairOpen,
	}
	if err := s.db.Create(&ticket).Error; err != nil {
		response.Error(c, "提交失败："+err.Error())
		return
	}
	s.db.Model(&model.Asset{}).Where("id = ?", req.AssetID).Update("status", model.AssetRepairing)
	service.Record(s.db, req.AssetID, "repair", u,
		fmt.Sprintf("提交报修工单 %s，故障现象：%s", ticket.TicketNo, req.Fault))

	s.notifier.NotifyRole([]string{"admin", "asset_manager"}, service.Message{
		Title: "新的设备报修工单",
		Content: fmt.Sprintf("%s 报修了「%s（%s）」，工单号 %s。故障现象：%s",
			u.RealName, asset.Name, asset.AssetNo, ticket.TicketNo, req.Fault),
		Type: "repair", Level: "warning", RefType: "repair", RefID: ticket.ID,
	})
	response.OK(c, gin.H{"message": "报修已提交，管理员会尽快处理", "ticket": ticket})
}

type handleRepairReq struct {
	Status   string  `json:"status" binding:"required"` // processing / resolved / closed
	Solution string  `json:"solution"`
	Cost     float64 `json:"cost"`
	// BackToStock=true 时修复完成把设备放回在库，否则退还给原领用人。
	BackToStock bool `json:"backToStock"`
}

func (s *Server) handleRepair(c *gin.Context) {
	var req handleRepairReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请选择处理结果")
		return
	}
	var t model.RepairTicket
	if err := s.db.Preload("Asset").First(&t, idParam(c)).Error; err != nil {
		response.NotFound(c, "工单不存在")
		return
	}
	if t.Status == model.RepairResolved || t.Status == model.RepairClosed {
		response.BadRequest(c, "该工单已完结")
		return
	}

	op := middleware.CurrentUser(c)
	now := time.Now()
	updates := map[string]any{
		"status": req.Status, "handler_id": op.ID,
		"solution": req.Solution, "cost": req.Cost,
	}
	if req.Status == model.RepairResolved || req.Status == model.RepairClosed {
		updates["resolved_at"] = now
	}
	if err := s.db.Model(&t).Updates(updates).Error; err != nil {
		response.Error(c, "保存失败")
		return
	}

	detail := fmt.Sprintf("工单 %s 处理进度更新为「%s」", t.TicketNo, repairStatusLabel(req.Status))
	if req.Status == model.RepairResolved {
		target := model.AssetInStock
		if !req.BackToStock && t.Asset != nil && t.Asset.HolderID != 0 {
			target = model.AssetInUse
		}
		s.db.Model(&model.Asset{}).Where("id = ?", t.AssetID).Update("status", target)
		detail = fmt.Sprintf("工单 %s 已修复，设备恢复为「%s」。处理方案：%s，维修费用 ¥%.2f",
			t.TicketNo, service.StatusLabel(target), req.Solution, req.Cost)
	}
	service.Record(s.db, t.AssetID, "repair", op, detail)

	s.notifier.Notify(t.ReporterID, service.Message{
		Title:   "报修工单进度更新",
		Content: detail,
		Type:    "repair", Level: "info", RefType: "repair", RefID: t.ID,
	}, req.Status == model.RepairResolved)
	response.OK(c, gin.H{"message": "工单已更新"})
}

func repairStatusLabel(s string) string {
	return map[string]string{
		model.RepairOpen: "待处理", model.RepairProcessing: "处理中",
		model.RepairResolved: "已修复", model.RepairClosed: "已关闭",
	}[s]
}
