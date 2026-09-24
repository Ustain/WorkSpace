package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"ams/internal/auth"
	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
)

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请输入用户名和密码")
		return
	}
	var u model.User
	if err := s.db.Preload("Role").Preload("Department").
		Where("username = ?", req.Username).First(&u).Error; err != nil {
		// 不区分"用户不存在"和"密码错误"，避免账号枚举。
		response.Unauthorized(c, "用户名或密码错误")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		response.Unauthorized(c, "用户名或密码错误")
		return
	}
	if !u.Enabled {
		response.Forbidden(c, "账号已被停用，请联系管理员")
		return
	}

	roleCode := ""
	if u.Role != nil {
		roleCode = u.Role.Code
	}
	token, expire, err := s.jwt.Issue(u.ID, u.Username, roleCode)
	if err != nil {
		response.Error(c, "生成登录凭证失败")
		return
	}
	now := time.Now()
	s.db.Model(&u).Update("last_login_at", now)

	s.db.Create(&model.AuditLog{
		UserID: u.ID, Username: u.Username, Module: "auth", Action: "登录",
		IP: c.ClientIP(), Method: "POST", Path: "/api/auth/login", CreatedAt: now,
	})

	response.OK(c, gin.H{
		"token":     token,
		"expiresAt": expire,
		"user":      profileOf(&u),
	})
}

func (s *Server) logout(c *gin.Context) { response.OK(c, gin.H{"message": "已退出登录"}) }

func (s *Server) profile(c *gin.Context) {
	response.OK(c, profileOf(middleware.CurrentUser(c)))
}

// profileOf 组装前端需要的用户视图，其中 permissions 决定菜单与按钮的可见性。
func profileOf(u *model.User) gin.H {
	perms := model.JSONSlice{}
	roleCode, roleName := "", ""
	if u.Role != nil {
		perms, roleCode, roleName = u.Role.Permissions, u.Role.Code, u.Role.Name
	}
	deptName := ""
	if u.Department != nil {
		deptName = u.Department.Name
	}
	return gin.H{
		"id": u.ID, "username": u.Username, "realName": u.RealName,
		"email": u.Email, "phone": u.Phone,
		"departmentId": u.DepartmentID, "departmentName": deptName,
		"roleId": u.RoleID, "roleCode": roleCode, "roleName": roleName,
		"permissions": perms, "lastLoginAt": u.LastLoginAt,
		"isAdmin": perms.Has(model.PermApplyApprove),
	}
}

type updateProfileReq struct {
	RealName string `json:"realName"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

func (s *Server) updateProfile(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req updateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数格式有误")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"real_name": req.RealName, "email": req.Email, "phone": req.Phone,
	}).Error; err != nil {
		response.Error(c, "保存失败")
		return
	}
	response.OK(c, gin.H{"message": "个人信息已更新"})
}

type changePasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

func (s *Server) changePassword(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "新密码长度至少 6 位")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.OldPassword) {
		response.BadRequest(c, "原密码不正确")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.Error(c, "密码加密失败")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", u.ID).Update("password_hash", hash).Error; err != nil {
		response.Error(c, "保存失败")
		return
	}
	response.OK(c, gin.H{"message": "密码已修改，请重新登录"})
}
