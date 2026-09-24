package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"ams/internal/auth"
	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
	"ams/internal/service"
)

// ---------- 设备类型 ----------

func (s *Server) listCategories(c *gin.Context) {
	var cats []model.Category
	if err := s.db.Order("sort ASC, id ASC").Find(&cats).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}
	// 附带每个类型下的设备数，管理页面能直接看出哪些类型在用。
	type stat struct {
		CategoryID uint
		N          int64
	}
	var stats []stat
	s.db.Model(&model.Asset{}).Select("category_id, COUNT(*) AS n").Group("category_id").Scan(&stats)
	counts := map[uint]int64{}
	for _, st := range stats {
		counts[st.CategoryID] = st.N
	}

	out := make([]gin.H, 0, len(cats))
	for _, cat := range cats {
		out = append(out, gin.H{
			"id": cat.ID, "name": cat.Name, "code": cat.Code, "icon": cat.Icon,
			"defaultLoanDays": cat.DefaultLoanDays, "sort": cat.Sort,
			"assetCount": counts[cat.ID],
		})
	}
	response.OK(c, out)
}

type categoryReq struct {
	Name            string `json:"name" binding:"required"`
	Code            string `json:"code"`
	Icon            string `json:"icon"`
	DefaultLoanDays int    `json:"defaultLoanDays"`
	Sort            int    `json:"sort"`
}

func (s *Server) saveCategory(c *gin.Context) {
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "类型名称不能为空")
		return
	}
	if req.DefaultLoanDays <= 0 {
		req.DefaultLoanDays = 180
	}
	if req.Code == "" {
		req.Code = strings.ToUpper(req.Name)
	}

	if id := idParam(c); id > 0 {
		if err := s.db.Model(&model.Category{}).Where("id = ?", id).Updates(map[string]any{
			"name": req.Name, "code": req.Code, "icon": req.Icon,
			"default_loan_days": req.DefaultLoanDays, "sort": req.Sort,
		}).Error; err != nil {
			response.BadRequest(c, "保存失败，类型名称可能重复")
			return
		}
		response.OK(c, gin.H{"message": "类型已更新"})
		return
	}
	cat := model.Category{
		Name: req.Name, Code: req.Code, Icon: req.Icon,
		DefaultLoanDays: req.DefaultLoanDays, Sort: req.Sort,
	}
	if err := s.db.Create(&cat).Error; err != nil {
		response.BadRequest(c, "新增失败，类型名称可能重复")
		return
	}
	response.OK(c, cat)
}

func (s *Server) deleteCategory(c *gin.Context) {
	id := idParam(c)
	var n int64
	s.db.Model(&model.Asset{}).Where("category_id = ?", id).Count(&n)
	if n > 0 {
		response.BadRequest(c, "该类型下仍有设备，请先转移或删除这些设备")
		return
	}
	if err := s.db.Delete(&model.Category{}, id).Error; err != nil {
		response.Error(c, "删除失败")
		return
	}
	response.OK(c, gin.H{"message": "类型已删除"})
}

// ---------- 部门 ----------

func (s *Server) listDepartments(c *gin.Context) {
	var depts []model.Department
	if err := s.db.Order("id ASC").Find(&depts).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}
	type stat struct {
		ID uint
		N  int64
	}
	var us, as []stat
	s.db.Model(&model.User{}).Select("department_id AS id, COUNT(*) AS n").Group("department_id").Scan(&us)
	s.db.Model(&model.Asset{}).Where("status = ?", model.AssetInUse).
		Select("holder_dept_id AS id, COUNT(*) AS n").Group("holder_dept_id").Scan(&as)

	userCnt, assetCnt := map[uint]int64{}, map[uint]int64{}
	for _, r := range us {
		userCnt[r.ID] = r.N
	}
	for _, r := range as {
		assetCnt[r.ID] = r.N
	}

	out := make([]gin.H, 0, len(depts))
	for _, d := range depts {
		out = append(out, gin.H{
			"id": d.ID, "name": d.Name, "code": d.Code, "leader": d.Leader, "remark": d.Remark,
			"userCount": userCnt[d.ID], "assetCount": assetCnt[d.ID],
		})
	}
	response.OK(c, out)
}

type deptReq struct {
	Name   string `json:"name" binding:"required"`
	Code   string `json:"code"`
	Leader string `json:"leader"`
	Remark string `json:"remark"`
}

func (s *Server) saveDepartment(c *gin.Context) {
	var req deptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "部门名称不能为空")
		return
	}
	if id := idParam(c); id > 0 {
		if err := s.db.Model(&model.Department{}).Where("id = ?", id).Updates(map[string]any{
			"name": req.Name, "code": req.Code, "leader": req.Leader, "remark": req.Remark,
		}).Error; err != nil {
			response.BadRequest(c, "保存失败，部门名称可能重复")
			return
		}
		response.OK(c, gin.H{"message": "部门已更新"})
		return
	}
	d := model.Department{Name: req.Name, Code: req.Code, Leader: req.Leader, Remark: req.Remark}
	if err := s.db.Create(&d).Error; err != nil {
		response.BadRequest(c, "新增失败，部门名称可能重复")
		return
	}
	response.OK(c, d)
}

func (s *Server) deleteDepartment(c *gin.Context) {
	id := idParam(c)
	var n int64
	s.db.Model(&model.User{}).Where("department_id = ?", id).Count(&n)
	if n > 0 {
		response.BadRequest(c, "该部门下仍有用户，请先调整用户所属部门")
		return
	}
	if err := s.db.Delete(&model.Department{}, id).Error; err != nil {
		response.Error(c, "删除失败")
		return
	}
	response.OK(c, gin.H{"message": "部门已删除"})
}

// ---------- 用户 ----------

func (s *Server) listUsers(c *gin.Context) {
	page, size, offset := pagination(c)
	q := s.db.Model(&model.User{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("username LIKE ? OR real_name LIKE ? OR email LIKE ? OR phone LIKE ?", like, like, like, like)
	}
	if v := c.Query("roleId"); v != "" {
		q = q.Where("role_id = ?", v)
	}
	if v := c.Query("deptId"); v != "" {
		q = q.Where("department_id = ?", v)
	}
	if v := c.Query("enabled"); v != "" {
		q = q.Where("enabled = ?", v == "true" || v == "1")
	}

	var total int64
	q.Count(&total)

	var users []model.User
	if err := q.Preload("Role").Preload("Department").
		Order("id ASC").Limit(size).Offset(offset).Find(&users).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}

	// 顺带带出每人当前持有的设备数，便于离职交接时核对。
	type stat struct {
		ID uint
		N  int64
	}
	var holds []stat
	s.db.Model(&model.Asset{}).Where("status = ?", model.AssetInUse).
		Select("holder_id AS id, COUNT(*) AS n").Group("holder_id").Scan(&holds)
	holdCnt := map[uint]int64{}
	for _, h := range holds {
		holdCnt[h.ID] = h.N
	}

	out := make([]gin.H, 0, len(users))
	for i := range users {
		u := users[i]
		v := profileOf(&u)
		v["enabled"] = u.Enabled
		v["holdingCount"] = holdCnt[u.ID]
		v["createdAt"] = u.CreatedAt
		out = append(out, v)
	}
	response.OKPage(c, out, total, page, size)
}

type userReq struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	RealName     string `json:"realName"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	DepartmentID uint   `json:"departmentId"`
	RoleID       uint   `json:"roleId"`
	Enabled      *bool  `json:"enabled"`
}

func (s *Server) createUser(c *gin.Context) {
	var req userReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数格式有误")
		return
	}
	if strings.TrimSpace(req.Username) == "" || req.RoleID == 0 {
		response.BadRequest(c, "用户名与角色为必填项")
		return
	}
	if len(req.Password) < 6 {
		response.BadRequest(c, "初始密码长度至少 6 位")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Error(c, "密码加密失败")
		return
	}
	u := model.User{
		Username: req.Username, PasswordHash: hash, RealName: req.RealName,
		Email: req.Email, Phone: req.Phone,
		DepartmentID: req.DepartmentID, RoleID: req.RoleID, Enabled: true,
	}
	if req.Enabled != nil {
		u.Enabled = *req.Enabled
	}
	if err := s.db.Create(&u).Error; err != nil {
		response.BadRequest(c, "新增失败，用户名可能已存在")
		return
	}
	s.notifier.Notify(u.ID, notifyWelcome(u.RealName), false)
	response.OK(c, gin.H{"message": "用户已创建", "id": u.ID})
}

func (s *Server) updateUser(c *gin.Context) {
	id := idParam(c)
	var req userReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数格式有误")
		return
	}
	me := middleware.CurrentUser(c)
	updates := map[string]any{
		"real_name": req.RealName, "email": req.Email, "phone": req.Phone,
		"department_id": req.DepartmentID,
	}
	if req.RoleID > 0 {
		updates["role_id"] = req.RoleID
	}
	if req.Enabled != nil {
		// 防止管理员把自己停用后再也进不来。
		if !*req.Enabled && id == me.ID {
			response.BadRequest(c, "不能停用当前登录的账号")
			return
		}
		updates["enabled"] = *req.Enabled
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Error(c, "保存失败")
		return
	}
	response.OK(c, gin.H{"message": "用户信息已更新"})
}

func (s *Server) resetPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	_ = c.ShouldBindJSON(&req)
	if len(req.Password) < 6 {
		req.Password = "ams@123456"
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Error(c, "密码加密失败")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", idParam(c)).
		Update("password_hash", hash).Error; err != nil {
		response.Error(c, "重置失败")
		return
	}
	response.OK(c, gin.H{"message": "密码已重置", "password": req.Password})
}

func (s *Server) deleteUser(c *gin.Context) {
	id := idParam(c)
	if me := middleware.CurrentUser(c); me.ID == id {
		response.BadRequest(c, "不能删除当前登录的账号")
		return
	}
	var holding int64
	s.db.Model(&model.Asset{}).Where("holder_id = ? AND status = ?", id, model.AssetInUse).Count(&holding)
	if holding > 0 {
		response.BadRequest(c, "该用户名下仍有领用中的设备，请先完成归还交接")
		return
	}
	if err := s.db.Delete(&model.User{}, id).Error; err != nil {
		response.Error(c, "删除失败")
		return
	}
	response.OK(c, gin.H{"message": "用户已删除"})
}

// ---------- 角色 ----------

func (s *Server) listRoles(c *gin.Context) {
	var roles []model.Role
	if err := s.db.Order("id ASC").Find(&roles).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}
	type stat struct {
		ID uint
		N  int64
	}
	var rows []stat
	s.db.Model(&model.User{}).Select("role_id AS id, COUNT(*) AS n").Group("role_id").Scan(&rows)
	cnt := map[uint]int64{}
	for _, r := range rows {
		cnt[r.ID] = r.N
	}

	out := make([]gin.H, 0, len(roles))
	for _, r := range roles {
		out = append(out, gin.H{
			"id": r.ID, "code": r.Code, "name": r.Name, "description": r.Description,
			"permissions": r.Permissions, "isSystem": r.IsSystem, "userCount": cnt[r.ID],
		})
	}
	response.OK(c, out)
}

func (s *Server) listPermissions(c *gin.Context) { response.OK(c, model.AllPermissions) }

type roleReq struct {
	Code        string   `json:"code"`
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (s *Server) saveRole(c *gin.Context) {
	var req roleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "角色名称不能为空")
		return
	}
	if id := idParam(c); id > 0 {
		var role model.Role
		if err := s.db.First(&role, id).Error; err != nil {
			response.NotFound(c, "角色不存在")
			return
		}
		updates := map[string]any{
			"name": req.Name, "description": req.Description,
			"permissions": model.JSONSlice(req.Permissions),
		}
		// 内置角色的 code 是权限判断的依据，不允许改。
		if !role.IsSystem && req.Code != "" {
			updates["code"] = req.Code
		}
		if err := s.db.Model(&model.Role{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			response.Error(c, "保存失败")
			return
		}
		response.OK(c, gin.H{"message": "角色已更新"})
		return
	}
	if req.Code == "" {
		response.BadRequest(c, "请填写角色编码")
		return
	}
	role := model.Role{
		Code: req.Code, Name: req.Name, Description: req.Description,
		Permissions: model.JSONSlice(req.Permissions),
	}
	if err := s.db.Create(&role).Error; err != nil {
		response.BadRequest(c, "新增失败，角色编码可能已存在")
		return
	}
	response.OK(c, role)
}

func (s *Server) deleteRole(c *gin.Context) {
	id := idParam(c)
	var role model.Role
	if err := s.db.First(&role, id).Error; err != nil {
		response.NotFound(c, "角色不存在")
		return
	}
	if role.IsSystem {
		response.BadRequest(c, "系统内置角色不允许删除")
		return
	}
	var n int64
	s.db.Model(&model.User{}).Where("role_id = ?", id).Count(&n)
	if n > 0 {
		response.BadRequest(c, "该角色下仍有用户，请先调整用户角色")
		return
	}
	if err := s.db.Delete(&model.Role{}, id).Error; err != nil {
		response.Error(c, "删除失败")
		return
	}
	response.OK(c, gin.H{"message": "角色已删除"})
}

// ---------- 系统设置 / 审计 / 邮件 / 定时任务 ----------

func (s *Server) getSettings(c *gin.Context) {
	response.OK(c, gin.H{
		"values":           s.settings.All(),
		"smtpEnabled":      s.cfg.SMTP.Enabled(),
		"smtpHost":         s.cfg.SMTP.Host,
		"reminderInterval": s.cfg.ReminderInterval.String(),
	})
}

func (s *Server) saveSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数格式有误")
		return
	}
	if err := s.settings.Save(req); err != nil {
		response.Error(c, "保存失败："+err.Error())
		return
	}
	response.OK(c, gin.H{"message": "系统设置已保存", "values": s.settings.All()})
}

func (s *Server) listAuditLogs(c *gin.Context) {
	page, size, offset := pagination(c)
	q := s.db.Model(&model.AuditLog{})
	if v := c.Query("username"); v != "" {
		q = q.Where("username LIKE ?", "%"+v+"%")
	}
	if v := c.Query("module"); v != "" {
		q = q.Where("module = ?", v)
	}
	if t := parseDay(c.Query("from"), false); t != nil {
		q = q.Where("created_at >= ?", t)
	}
	if t := parseDay(c.Query("to"), true); t != nil {
		q = q.Where("created_at <= ?", t)
	}

	var total int64
	q.Count(&total)

	var logs []model.AuditLog
	if err := q.Order("id DESC").Limit(size).Offset(offset).Find(&logs).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}
	response.OKPage(c, logs, total, page, size)
}

func (s *Server) listEmails(c *gin.Context) {
	page, size, offset := pagination(c)
	q := s.db.Model(&model.EmailLog{})
	if v := c.Query("to"); v != "" {
		q = q.Where("\"to\" LIKE ?", "%"+v+"%")
	}
	var total int64
	q.Count(&total)

	var logs []model.EmailLog
	if err := q.Order("id DESC").Limit(size).Offset(offset).Find(&logs).Error; err != nil {
		response.Error(c, "查询失败："+err.Error())
		return
	}
	response.OKPage(c, logs, total, page, size)
}

// runReminder 手动触发一次到期扫描。演示时不用等定时器，点一下就能看到提醒产生。
func (s *Server) runReminder(c *gin.Context) {
	res, err := s.reminder.Run()
	if err != nil {
		response.Error(c, "执行失败："+err.Error())
		return
	}
	response.OK(c, gin.H{
		"message": "到期扫描已执行",
		"result":  res,
	})
}

func notifyWelcome(name string) service.Message {
	return service.Message{
		Title: "欢迎加入资产管理系统",
		Content: name + "，您的账号已创建。可在「可领用设备」中挑选设备发起领用申请，" +
			"审批结果会通过站内信与邮件通知您。",
		Type: "system", Level: "info",
	}
}
