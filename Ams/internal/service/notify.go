package service

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"gorm.io/gorm"

	"ams/internal/config"
	"ams/internal/model"
)

// Notifier 统一出口：站内信必发，邮件按需异步发。
//
// 通知策略（对应需求 3）：
//   - 管理员 / 资产管理员：只发站内信，避免审批类高频消息刷屏邮箱；
//   - 普通领用用户：站内信 + 邮件双通道。
type Notifier struct {
	db       *gorm.DB
	settings *SettingService
	smtp     config.SMTPConfig
}

func NewNotifier(db *gorm.DB, settings *SettingService, smtpCfg config.SMTPConfig) *Notifier {
	return &Notifier{db: db, settings: settings, smtp: smtpCfg}
}

type Message struct {
	Title   string
	Content string
	Type    string // approval / due / overdue / repair / system
	Level   string // info / success / warning / danger
	RefType string
	RefID   uint
}

// Notify 给指定用户推送。withEmail 表示"业务上希望也发邮件"，
// 最终是否真发还要看该用户角色与系统开关。
func (n *Notifier) Notify(userID uint, msg Message, withEmail bool) {
	if userID == 0 {
		return
	}
	note := model.Notification{
		UserID: userID, Title: msg.Title, Content: msg.Content,
		Type: defaultStr(msg.Type, "system"), Level: defaultStr(msg.Level, "info"),
		RefType: msg.RefType, RefID: msg.RefID, CreatedAt: time.Now(),
	}
	if err := n.db.Create(&note).Error; err != nil {
		log.Printf("[notify] 站内信写入失败 user=%d: %v", userID, err)
		return
	}
	if !withEmail || !n.settings.Bool("email_notify", true) {
		return
	}

	var u model.User
	if err := n.db.Preload("Role").First(&u, userID).Error; err != nil || u.Email == "" {
		return
	}
	// 管理类角色只走站内信。
	if u.Role != nil && (u.Role.Code == "admin" || u.Role.Code == "asset_manager") {
		return
	}
	go n.sendMail(u.Email, msg.Title, buildHTML(u.RealName, msg))
}

// NotifyRole 按角色广播，用于"有新申请待审批"这类需要通知全体管理员的场景。
func (n *Notifier) NotifyRole(roleCodes []string, msg Message) {
	var users []model.User
	if err := n.db.Joins("JOIN roles ON roles.id = users.role_id").
		Where("roles.code IN ? AND users.enabled = ?", roleCodes, true).
		Find(&users).Error; err != nil {
		log.Printf("[notify] 按角色查询用户失败: %v", err)
		return
	}
	for _, u := range users {
		n.Notify(u.ID, msg, false)
	}
}

// sendMail 未配置 SMTP 时进入模拟投递：内容照常落库，前端「邮件记录」可查，
// 这样演示环境不依赖真实邮箱也能完整展示通知链路。
func (n *Notifier) sendMail(to, subject, body string) {
	entry := model.EmailLog{To: to, Subject: subject, Body: body, CreatedAt: time.Now()}
	if !n.smtp.Enabled() {
		entry.Status = "mocked"
		n.db.Create(&entry)
		log.Printf("[mail:mock] to=%s subject=%s", to, subject)
		return
	}
	if err := n.deliver(to, subject, body); err != nil {
		entry.Status, entry.Error = "failed", err.Error()
		log.Printf("[mail:fail] to=%s err=%v", to, err)
	} else {
		entry.Status = "sent"
	}
	n.db.Create(&entry)
}

func (n *Notifier) deliver(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", n.smtp.Host, n.smtp.Port)
	msg := buildMIME(n.smtp.From, to, subject, body)
	a := smtp.PlainAuth("", n.smtp.Username, n.smtp.Password, n.smtp.Host)

	if n.smtp.Port != 465 { // 非 SMTPS，走 STARTTLS
		return smtp.SendMail(addr, a, n.smtp.From, []string{to}, msg)
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: n.smtp.Host})
	if err != nil {
		return err
	}
	defer conn.Close()
	cl, err := smtp.NewClient(conn, n.smtp.Host)
	if err != nil {
		return err
	}
	defer cl.Quit()
	if err := cl.Auth(a); err != nil {
		return err
	}
	if err := cl.Mail(n.smtp.From); err != nil {
		return err
	}
	if err := cl.Rcpt(to); err != nil {
		return err
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func buildMIME(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

var levelColor = map[string]string{
	"danger": "#f56c6c", "warning": "#e6a23c", "success": "#67c23a", "info": "#409eff",
}

func buildHTML(name string, msg Message) string {
	color := levelColor[msg.Level]
	if color == "" {
		color = "#409eff"
	}
	return fmt.Sprintf(`<div style="font-family:-apple-system,Segoe UI,Microsoft YaHei,sans-serif;max-width:640px;margin:0 auto;border:1px solid #ebeef5;border-radius:10px;overflow:hidden">
  <div style="background:%s;color:#fff;padding:18px 24px;font-size:17px;font-weight:600">%s</div>
  <div style="padding:24px;color:#303133;line-height:1.85;font-size:14px">
    <p style="margin:0 0 12px">%s，您好：</p>
    <p style="margin:0 0 18px">%s</p>
    <p style="margin:0;color:#909399;font-size:12px">本邮件由资产管理系统自动发送，请勿直接回复。如需办理业务请登录系统操作。</p>
  </div>
  <div style="background:#fafafa;padding:12px 24px;color:#c0c4cc;font-size:12px">发送时间：%s</div>
</div>`, color, msg.Title, name, msg.Content, time.Now().Format("2006-01-02 15:04:05"))
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
