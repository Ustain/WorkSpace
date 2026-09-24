package service_test

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"ams/internal/config"
	"ams/internal/model"
	"ams/internal/service"
	"ams/internal/store"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// fixture 手工搭一套最小数据：一个管理员、一个普通用户、一台领用中的设备。
// 不用 store.Seed 是为了让断言只受本用例控制。
type fixture struct {
	db       *gorm.DB
	reminder *service.Reminder
	admin    model.User
	holder   model.User
	asset    model.Asset
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	adminRole := model.Role{Code: "admin", Name: "管理员", Permissions: model.JSONSlice{"*"}}
	userRole := model.Role{Code: "user", Name: "普通用户", Permissions: model.JSONSlice{"asset:view"}}
	if err := db.Create(&[]model.Role{adminRole, userRole}).Error; err != nil {
		t.Fatalf("创建角色失败: %v", err)
	}
	db.Where("code = ?", "admin").First(&adminRole)
	db.Where("code = ?", "user").First(&userRole)

	admin := model.User{Username: "admin", RealName: "管理员", Email: "admin@test.local",
		RoleID: adminRole.ID, Enabled: true}
	holder := model.User{Username: "u1", RealName: "张三", Email: "u1@test.local",
		RoleID: userRole.ID, Enabled: true}
	if err := db.Create(&[]model.User{admin, holder}).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	db.Where("username = ?", "admin").First(&admin)
	db.Where("username = ?", "u1").First(&holder)

	cat := model.Category{Name: "笔记本电脑", Code: "NB", DefaultLoanDays: 180}
	db.Create(&cat)

	borrowed := time.Now().AddDate(0, 0, -100)
	due := time.Now().AddDate(0, 0, 3) // 3 天后到期，落在默认 7 天提醒窗口内
	asset := model.Asset{
		AssetNo: "NB-2026-0001", Name: "测试笔记本", CategoryID: cat.ID,
		Status: model.AssetInUse, HolderID: holder.ID,
		BorrowedAt: &borrowed, DueAt: &due,
	}
	db.Create(&asset)

	settings := service.NewSettingService(db)
	settings.Save(map[string]string{"remind_before_days": "7", "email_notify": "true"})
	notifier := service.NewNotifier(db, settings, config.SMTPConfig{})

	return &fixture{
		db: db, reminder: service.NewReminder(db, notifier, settings),
		admin: admin, holder: holder, asset: asset,
	}
}

func (f *fixture) countNotices(userID uint, typ string) int64 {
	var n int64
	q := f.db.Model(&model.Notification{}).Where("user_id = ?", userID)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	q.Count(&n)
	return n
}

func (f *fixture) setDue(t time.Time) {
	f.db.Model(&model.Asset{}).Where("id = ?", f.asset.ID).Update("due_at", t)
}

// waitForMail 邮件是异步发的，给它一点时间落库。
func (f *fixture) waitForMail(t *testing.T, want int64) int64 {
	t.Helper()
	var n int64
	for i := 0; i < 50; i++ {
		f.db.Model(&model.EmailLog{}).Count(&n)
		if n >= want {
			return n
		}
		time.Sleep(20 * time.Millisecond)
	}
	return n
}

func TestReminderNotifiesHolderAndAdmin(t *testing.T) {
	f := newFixture(t)

	res, err := f.reminder.Run()
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if res.DueSoon != 1 {
		t.Errorf("应识别出 1 台临期设备，实际 %d", res.DueSoon)
	}
	if res.Notified != 1 {
		t.Errorf("应推送 1 条提醒，实际 %d", res.Notified)
	}

	if got := f.countNotices(f.holder.ID, "due"); got != 1 {
		t.Errorf("领用人应收到 1 条到期站内信，实际 %d", got)
	}
	// 管理员拿到的是汇总消息，不是每台设备一条。
	if got := f.countNotices(f.admin.ID, "due"); got != 1 {
		t.Errorf("管理员应收到 1 条汇总消息，实际 %d", got)
	}

	// 普通用户额外发邮件；管理员不发。
	if got := f.waitForMail(t, 1); got != 1 {
		t.Errorf("应为普通用户产生 1 封邮件，实际 %d", got)
	}
	var mail model.EmailLog
	f.db.First(&mail)
	if mail.To != f.holder.Email {
		t.Errorf("邮件收件人应为领用人 %s，实际 %s", f.holder.Email, mail.To)
	}
	if mail.Status != "mocked" {
		t.Errorf("未配置 SMTP 时应为模拟投递，实际 %s", mail.Status)
	}
}

func TestReminderIsIdempotentWithinOnePeriod(t *testing.T) {
	f := newFixture(t)

	f.reminder.Run()
	first := f.countNotices(f.holder.ID, "due")

	// 连跑三次，不应产生重复打扰。
	for i := 0; i < 3; i++ {
		res, _ := f.reminder.Run()
		if res.Notified != 0 {
			t.Fatalf("第 %d 次重复扫描不应再推送，实际推送 %d 条", i+2, res.Notified)
		}
	}
	if got := f.countNotices(f.holder.ID, "due"); got != first {
		t.Errorf("重复扫描后站内信数量应保持 %d，实际 %d", first, got)
	}
}

func TestReminderEscalatesToOverdue(t *testing.T) {
	f := newFixture(t)

	f.reminder.Run() // 先产生一次临期提醒
	f.setDue(time.Now().AddDate(0, 0, -5))

	res, err := f.reminder.Run()
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if res.Overdue != 1 {
		t.Errorf("应识别出 1 台逾期设备，实际 %d", res.Overdue)
	}
	if got := f.countNotices(f.holder.ID, "overdue"); got != 1 {
		t.Errorf("领用人应收到 1 条逾期告警，实际 %d", got)
	}

	// 逾期同样只告警一次。
	f.reminder.Run()
	if got := f.countNotices(f.holder.ID, "overdue"); got != 1 {
		t.Errorf("逾期告警不应重复推送，实际 %d 条", got)
	}
}

// 续借/归还会清空提醒标记，新的借用周期必须能重新提醒，
// 否则一台反复续借的设备就再也收不到到期通知了。
func TestReminderResetsAfterRenew(t *testing.T) {
	f := newFixture(t)

	f.reminder.Run()
	if got := f.countNotices(f.holder.ID, "due"); got != 1 {
		t.Fatalf("首轮应提醒 1 次，实际 %d", got)
	}

	f.db.Model(&model.Asset{}).Where("id = ?", f.asset.ID).Updates(map[string]any{
		"due_at": time.Now().AddDate(0, 0, 2), "reminded_at": nil, "overdue_at": nil,
	})

	res, _ := f.reminder.Run()
	if res.Notified != 1 {
		t.Errorf("续借后新周期应重新提醒，实际推送 %d 条", res.Notified)
	}
	if got := f.countNotices(f.holder.ID, "due"); got != 2 {
		t.Errorf("续借后领用人应累计收到 2 条到期提醒，实际 %d", got)
	}
}

func TestReminderIgnoresAssetsOutsideWindow(t *testing.T) {
	f := newFixture(t)
	f.setDue(time.Now().AddDate(0, 0, 60)) // 远在提醒窗口之外

	res, err := f.reminder.Run()
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if res.DueSoon != 0 || res.Notified != 0 {
		t.Errorf("窗口外的设备不应触发提醒，实际 dueSoon=%d notified=%d", res.DueSoon, res.Notified)
	}
	if got := f.countNotices(f.holder.ID, ""); got != 0 {
		t.Errorf("不应产生任何站内信，实际 %d 条", got)
	}
}

func TestReminderRespectsSettingWindow(t *testing.T) {
	f := newFixture(t)
	f.setDue(time.Now().AddDate(0, 0, 20))

	if res, _ := f.reminder.Run(); res.Notified != 0 {
		t.Fatalf("默认 7 天窗口下不应提醒 20 天后到期的设备")
	}

	// 把窗口放宽到 30 天，同一台设备就该进入提醒范围。
	settings := service.NewSettingService(f.db)
	if err := settings.Save(map[string]string{"remind_before_days": "30"}); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	notifier := service.NewNotifier(f.db, settings, config.SMTPConfig{})
	rm := service.NewReminder(f.db, notifier, settings)

	if res, _ := rm.Run(); res.Notified != 1 {
		t.Errorf("放宽窗口后应提醒 1 条，实际 %d", res.Notified)
	}
}
