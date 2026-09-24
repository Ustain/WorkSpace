package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
	"ams/internal/service"
)

func (s *Server) listApplications(c *gin.Context) {
	page, size, offset := pagination(c)
	u := middleware.CurrentUser(c)

	q := s.db.Model(&model.Application{})
	// 没有"查看全部"权限的人只能看到自己的单子；显式 mine=1 时任何人都只看自己的。
	if !middleware.HasPerm(u, model.PermApplyViewAll) || c.Query("mine") == "1" {
		q = q.Where("applicant_id = ?", u.ID)
	}
	if v := multiQuery(c, "status"); len(v) > 0 {
		q = q.Where("status IN ?", v)
	}
	if v := multiQuery(c, "type"); len(v) > 0 {
		q = q.Where("type IN ?", v)
	}
	if v := c.Query("applicantId"); v != "" {
		q = q.Where("applicant_id = ?", v)
	}
	if v := c.Query("deptId"); v != "" {
		q = q.Where("dept_id = ?", v)
	}
	if t := parseDay(c.Query("createdFrom"), false); t != nil {
		q = q.Where("created_at >= ?", t)
	}
	if t := parseDay(c.Query("createdTo"), true); t != nil {
		q = q.Where("created_at <= ?", t)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("apply_no LIKE ? OR reason LIKE ?", like, like)
	}

	var total int64
	q.Count(&total)

	var apps []model.Application
	if err := q.Preload("Asset").Preload("Asset.Category").
		Preload("Applicant").Preload("Applicant.Department").Preload("Approver").
		// 待审批排最前，管理员一进来就先看到要处理的。
		Order("CASE status WHEN 'pending' THEN 0 ELSE 1 END, created_at DESC").
		Limit(size).Offset(offset).Find(&apps).Error; err != nil {
		response.Error(c, "查询失败："+err.Error())
		return
	}
	response.OKPage(c, apps, total, page, size)
}

func (s *Server) getApplication(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var app model.Application
	if err := s.db.Preload("Asset").Preload("Asset.Category").
		Preload("Applicant").Preload("Approver").First(&app, idParam(c)).Error; err != nil {
		response.NotFound(c, "申请不存在")
		return
	}
	if app.ApplicantID != u.ID && !middleware.HasPerm(u, model.PermApplyViewAll) {
		response.Forbidden(c, "无权查看该申请")
		return
	}
	response.OK(c, app)
}

type createApplyReq struct {
	Type     string `json:"type" binding:"required"`
	AssetID  uint   `json:"assetId" binding:"required"`
	Reason   string `json:"reason" binding:"required"`
	LoanDays int    `json:"loanDays"`
}

func (s *Server) createApplication(c *gin.Context) {
	var req createApplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请填写完整的申请信息（设备与事由为必填）")
		return
	}
	app, err := s.workflow.Submit(service.SubmitInput{
		Type: req.Type, AssetID: req.AssetID, Reason: req.Reason,
		LoanDays: req.LoanDays, Applicant: middleware.CurrentUser(c),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "申请已提交，请等待管理员审批", "application": app})
}

type approveReq struct {
	Comment string `json:"comment"`
}

func (s *Server) approveApplication(c *gin.Context) {
	var req approveReq
	_ = c.ShouldBindJSON(&req)
	app, err := s.workflow.Approve(idParam(c), middleware.CurrentUser(c), req.Comment)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "审批通过，已通知申请人", "application": app})
}

func (s *Server) rejectApplication(c *gin.Context) {
	var req approveReq
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Comment) == "" {
		response.BadRequest(c, "驳回时请填写理由，便于申请人了解原因")
		return
	}
	app, err := s.workflow.Reject(idParam(c), middleware.CurrentUser(c), req.Comment)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "已驳回并通知申请人", "application": app})
}

func (s *Server) cancelApplication(c *gin.Context) {
	if err := s.workflow.Cancel(idParam(c), middleware.CurrentUser(c)); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "申请已撤销"})
}
