package api

import (
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ams/internal/auth"
	"ams/internal/config"
	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
	"ams/internal/service"
)

type Server struct {
	db       *gorm.DB
	cfg      *config.Config
	jwt      *auth.Manager
	settings *service.SettingService
	notifier *service.Notifier
	workflow *service.Workflow
	reminder *service.Reminder
}

func NewServer(db *gorm.DB, cfg *config.Config, jwt *auth.Manager, st *service.SettingService,
	n *service.Notifier, wf *service.Workflow, rm *service.Reminder) *Server {
	return &Server{db: db, cfg: cfg, jwt: jwt, settings: st, notifier: n, workflow: wf, reminder: rm}
}

// Router 装配全部路由。分三层：公开接口 / 登录即可访问 / 需要具体权限点。
func (s *Server) Router(webFS fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), middleware.Recovery())

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"status": "ok", "time": time.Now()})
	})
	api.GET("/meta", s.meta)
	api.POST("/auth/login", s.login)

	authed := api.Group("", middleware.Auth(s.db, s.jwt), middleware.Audit(s.db))
	{
		authed.GET("/auth/profile", s.profile)
		authed.PUT("/auth/profile", s.updateProfile)
		authed.PUT("/auth/password", s.changePassword)
		authed.POST("/auth/logout", s.logout)

		// 通知中心
		authed.GET("/notifications", s.listNotifications)
		authed.GET("/notifications/unread-count", s.unreadCount)
		authed.POST("/notifications/:id/read", s.readNotification)
		authed.POST("/notifications/read-all", s.readAllNotifications)
		authed.DELETE("/notifications/:id", s.deleteNotification)

		// 设备
		authed.GET("/assets", middleware.RequirePerm(model.PermAssetView), s.listAssets)
		authed.GET("/assets/options", s.assetOptions)
		authed.GET("/assets/export", middleware.RequirePerm(model.PermAssetExport), s.exportAssets)
		authed.GET("/assets/import/template", middleware.RequirePerm(model.PermAssetImport), s.importTemplate)
		authed.POST("/assets/import", middleware.RequirePerm(model.PermAssetImport), s.importAssets)
		authed.POST("/assets/batch", middleware.RequirePerm(model.PermAssetEdit), s.batchAssets)
		authed.GET("/assets/:id", middleware.RequirePerm(model.PermAssetView), s.getAsset)
		authed.GET("/assets/:id/records", middleware.RequirePerm(model.PermAssetView), s.assetRecords)
		authed.GET("/assets/:id/qrcode", s.assetQRCode)
		authed.POST("/assets", middleware.RequirePerm(model.PermAssetEdit), s.createAsset)
		authed.PUT("/assets/:id", middleware.RequirePerm(model.PermAssetEdit), s.updateAsset)
		authed.DELETE("/assets/:id", middleware.RequirePerm(model.PermAssetDelete), s.deleteAsset)

		// 申请与审批
		authed.GET("/applications", s.listApplications)
		authed.POST("/applications", middleware.RequirePerm(model.PermApplyCreate), s.createApplication)
		authed.GET("/applications/:id", s.getApplication)
		authed.POST("/applications/:id/approve", middleware.RequirePerm(model.PermApplyApprove), s.approveApplication)
		authed.POST("/applications/:id/reject", middleware.RequirePerm(model.PermApplyApprove), s.rejectApplication)
		authed.POST("/applications/:id/cancel", s.cancelApplication)

		// 报修工单
		authed.GET("/repairs", s.listRepairs)
		authed.POST("/repairs", s.createRepair)
		authed.POST("/repairs/:id/handle", middleware.RequirePerm(model.PermRepairHandle), s.handleRepair)

		// 统计
		authed.GET("/stats/overview", s.statsOverview)
		authed.GET("/stats/charts", s.statsCharts)
		authed.GET("/stats/my", s.statsMine)

		// 组织与系统管理
		authed.GET("/categories", s.listCategories)
		authed.POST("/categories", middleware.RequirePerm(model.PermAssetEdit), s.saveCategory)
		authed.PUT("/categories/:id", middleware.RequirePerm(model.PermAssetEdit), s.saveCategory)
		authed.DELETE("/categories/:id", middleware.RequirePerm(model.PermAssetEdit), s.deleteCategory)

		authed.GET("/departments", s.listDepartments)
		authed.POST("/departments", middleware.RequirePerm(model.PermDeptManage), s.saveDepartment)
		authed.PUT("/departments/:id", middleware.RequirePerm(model.PermDeptManage), s.saveDepartment)
		authed.DELETE("/departments/:id", middleware.RequirePerm(model.PermDeptManage), s.deleteDepartment)

		authed.GET("/users", middleware.RequirePerm(model.PermUserManage), s.listUsers)
		authed.POST("/users", middleware.RequirePerm(model.PermUserManage), s.createUser)
		authed.PUT("/users/:id", middleware.RequirePerm(model.PermUserManage), s.updateUser)
		authed.POST("/users/:id/reset-password", middleware.RequirePerm(model.PermUserManage), s.resetPassword)
		authed.DELETE("/users/:id", middleware.RequirePerm(model.PermUserManage), s.deleteUser)

		authed.GET("/roles", s.listRoles)
		authed.GET("/roles/permissions", s.listPermissions)
		authed.POST("/roles", middleware.RequirePerm(model.PermRoleManage), s.saveRole)
		authed.PUT("/roles/:id", middleware.RequirePerm(model.PermRoleManage), s.saveRole)
		authed.DELETE("/roles/:id", middleware.RequirePerm(model.PermRoleManage), s.deleteRole)

		authed.GET("/settings", middleware.RequirePerm(model.PermSettingManage), s.getSettings)
		authed.PUT("/settings", middleware.RequirePerm(model.PermSettingManage), s.saveSettings)
		authed.GET("/audit-logs", middleware.RequirePerm(model.PermAuditView), s.listAuditLogs)
		authed.GET("/emails", middleware.RequirePerm(model.PermSettingManage), s.listEmails)
		authed.POST("/jobs/reminder/run", middleware.RequirePerm(model.PermSettingManage), s.runReminder)
	}

	s.mountWeb(r, webFS)
	return r
}

// mountWeb 提供内嵌前端；未匹配到静态文件时回落到 index.html，支持前端路由直接刷新。
func (s *Server) mountWeb(r *gin.Engine, webFS fs.FS) {
	fileServer := http.FileServer(http.FS(webFS))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			response.NotFound(c, "接口不存在")
			return
		}
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		// 第三方库带版本号且不会变，长缓存；业务代码每次升级都要立刻生效，禁缓存。
		if strings.HasPrefix(p, "vendor/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache, must-revalidate")
		}

		if _, err := fs.Stat(webFS, p); err != nil {
			data, err := fs.ReadFile(webFS, "index.html")
			if err != nil {
				response.NotFound(c, "页面不存在")
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

// meta 登录页需要的公开信息：系统名称与演示账号。
func (s *Server) meta(c *gin.Context) {
	response.OK(c, gin.H{
		"systemName": s.settings.Get("system_name", "IT 资产管理系统"),
		"orgName":    s.settings.Get("org_name", ""),
		"demoAccounts": []gin.H{
			{"username": "admin", "password": "admin123", "role": "系统管理员", "desc": "全部功能：台账、审批、统计、用户与系统配置"},
			{"username": "assetmgr", "password": "admin123", "role": "资产管理员", "desc": "台账维护与审批，无用户/角色管理权限"},
			{"username": "zhouheng", "password": "user123", "role": "普通用户", "desc": "申请领用、归还续借、报修与我的设备"},
		},
	})
}

// ---------- 通用小工具 ----------

func pagination(c *gin.Context) (page, size, offset int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 20
	}
	return page, size, (page - 1) * size
}

func idParam(c *gin.Context) uint {
	n, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(n)
}

// parseDay 解析 yyyy-MM-dd；endOfDay=true 时取当天 23:59:59，
// 这样"到期时间 <= 某天"的范围筛选能把当天全部包含进来。
func parseDay(s string, endOfDay bool) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", s[:min(10, len(s))], time.Local)
	if err != nil {
		return nil
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Second)
	}
	return &t
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
