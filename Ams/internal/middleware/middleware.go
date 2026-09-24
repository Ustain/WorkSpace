package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ams/internal/auth"
	"ams/internal/model"
	"ams/internal/response"
)

const (
	ctxUser = "ams.user"
)

// Auth 校验 Bearer Token 并把完整用户（含角色/部门）塞进上下文。
// 每次请求都回查数据库，这样禁用账号或调整权限可以立即生效，不用等 token 过期。
func Auth(db *gorm.DB, mgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.Unauthorized(c, "未登录或登录已失效")
			return
		}
		claims, err := mgr.Parse(token)
		if err != nil {
			response.Unauthorized(c, "登录已失效，请重新登录")
			return
		}
		var u model.User
		if err := db.Preload("Role").Preload("Department").First(&u, claims.UserID).Error; err != nil {
			response.Unauthorized(c, "账号不存在")
			return
		}
		if !u.Enabled {
			response.Forbidden(c, "账号已被停用，请联系管理员")
			return
		}
		c.Set(ctxUser, &u)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// 导出、二维码等场景走浏览器直接下载，无法带自定义头，允许用 query 传 token。
	return c.Query("token")
}

// CurrentUser 取出当前登录用户；未登录返回 nil。
func CurrentUser(c *gin.Context) *model.User {
	if v, ok := c.Get(ctxUser); ok {
		if u, ok := v.(*model.User); ok {
			return u
		}
	}
	return nil
}

// HasPerm 判断用户是否持有指定权限点。
func HasPerm(u *model.User, perm string) bool {
	return u != nil && u.Role != nil && u.Role.Permissions.Has(perm)
}

// RequirePerm 声明式权限校验，用在路由上一目了然。
func RequirePerm(perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		for _, p := range perms {
			if HasPerm(u, p) {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "没有权限执行该操作")
	}
}

// bodyCapture 让审计中间件能在 handler 写完响应后判断是否成功。
type bodyCapture struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w bodyCapture) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

// 这些前缀属于高频只读接口，记审计只会淹没真正重要的操作。
var auditSkipPrefix = []string{"/api/notifications", "/api/stats", "/api/auth/profile"}

// Audit 记录所有写操作（POST/PUT/DELETE）的执行人、目标与入参。
func Audit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && !strings.HasSuffix(c.Request.URL.Path, "/export") {
			c.Next()
			return
		}
		for _, p := range auditSkipPrefix {
			if strings.HasPrefix(c.Request.URL.Path, p) {
				c.Next()
				return
			}
		}

		var reqBody []byte
		if c.Request.Body != nil && c.ContentType() == "application/json" {
			reqBody, _ = io.ReadAll(io.LimitReader(c.Request.Body, 4096))
			c.Request.Body = io.NopCloser(bytes.NewReader(reqBody))
		}

		w := bodyCapture{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = w
		c.Next()

		if c.Writer.Status() >= 400 {
			return
		}
		u := CurrentUser(c)
		entry := model.AuditLog{
			Module:    moduleOf(c.Request.URL.Path),
			Action:    actionLabel(c),
			Target:    c.Param("id"),
			Detail:    sanitize(reqBody),
			IP:        c.ClientIP(),
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			CreatedAt: time.Now(),
		}
		if u != nil {
			entry.UserID, entry.Username = u.ID, u.Username
		}
		if err := db.Create(&entry).Error; err != nil {
			log.Printf("[audit] 写入失败: %v", err)
		}
	}
}

// sanitize 去掉入参里的密码字段，审计日志不应留下明文口令。
func sanitize(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return truncate(string(body), 1000)
	}
	for _, k := range []string{"password", "oldPassword", "newPassword", "smtpPass"} {
		if _, ok := m[k]; ok {
			m[k] = "***"
		}
	}
	b, _ := json.Marshal(m)
	return truncate(string(b), 1000)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func moduleOf(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "-"
}

var actionNames = map[string]string{
	"POST": "新增/提交", "PUT": "修改", "DELETE": "删除", "GET": "导出",
}

func actionLabel(c *gin.Context) string {
	name := actionNames[c.Request.Method]
	// 形如 /api/applications/12/approve 的动作后缀更能说明做了什么。
	parts := strings.Split(strings.Trim(c.Request.URL.Path, "/"), "/")
	if len(parts) >= 3 {
		if last := parts[len(parts)-1]; !isNumeric(last) {
			return name + ":" + last
		}
	}
	return name
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Recovery 兜底 panic，返回统一错误结构而不是空连接。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic] %s %s: %v", c.Request.Method, c.Request.URL.Path, r)
				response.Error(c, "服务内部错误")
			}
		}()
		c.Next()
	}
}
