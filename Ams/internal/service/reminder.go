package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"ams/internal/model"
)

// Reminder 负责"设备领用到期"的自动提醒（对应需求 3）。
//
// 两类事件各自只推一次，靠 assets.reminded_at / overdue_at 打标去重；
// 设备归还或续借时这两个标记会被清空，进入下一个借用周期后可再次提醒。
type Reminder struct {
	db       *gorm.DB
	notifier *Notifier
	settings *SettingService
}

func NewReminder(db *gorm.DB, n *Notifier, s *SettingService) *Reminder {
	return &Reminder{db: db, notifier: n, settings: s}
}

// Result 一次扫描的结果，手动触发接口会把它返回给前端展示。
type Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	DueSoon     int       `json:"dueSoon"`
	Overdue     int       `json:"overdue"`
	Notified    int       `json:"notified"`
	AdminDigest bool      `json:"adminDigest"`
}

// Start 后台周期扫描。启动后立即跑一次，让演示时打开页面就有提醒数据。
func (r *Reminder) Start(ctx context.Context, interval time.Duration) {
	go func() {
		if res, err := r.Run(); err != nil {
			log.Printf("[reminder] 首次扫描失败: %v", err)
		} else {
			log.Printf("[reminder] 首次扫描完成: 临期 %d, 超期 %d, 推送 %d", res.DueSoon, res.Overdue, res.Notified)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := r.Run(); err != nil {
					log.Printf("[reminder] 扫描失败: %v", err)
				}
			}
		}
	}()
}

func (r *Reminder) Run() (*Result, error) {
	now := time.Now()
	before := r.settings.Int("remind_before_days", 7)
	threshold := now.AddDate(0, 0, before)
	res := &Result{ScannedAt: now}

	var assets []model.Asset
	err := r.db.Preload("Holder").Preload("Category").
		Where("status = ? AND due_at IS NOT NULL AND due_at <= ?", model.AssetInUse, threshold).
		Find(&assets).Error
	if err != nil {
		return nil, err
	}

	var dueSoonList, overdueList []model.Asset
	for i := range assets {
		a := assets[i]
		if a.DueAt.Before(now) {
			res.Overdue++
			if a.OverdueAt == nil {
				overdueList = append(overdueList, a)
			}
		} else {
			res.DueSoon++
			if a.RemindedAt == nil {
				dueSoonList = append(dueSoonList, a)
			}
		}
	}

	for _, a := range dueSoonList {
		days := int(a.DueAt.Sub(now).Hours()/24) + 1
		r.notifier.Notify(a.HolderID, Message{
			Title: fmt.Sprintf("设备领用即将到期（剩余 %d 天）", days),
			Content: fmt.Sprintf("您领用的「%s（资产编号 %s）」将于 %s 到期。请及时归还，若仍需继续使用，请在系统中提交续借申请。",
				a.Name, a.AssetNo, a.DueAt.Format("2006年01月02日")),
			Type: "due", Level: "warning", RefType: "asset", RefID: a.ID,
		}, true) // 普通用户：站内信 + 邮件
		r.db.Model(&model.Asset{}).Where("id = ?", a.ID).Update("reminded_at", now)
		res.Notified++
	}

	for _, a := range overdueList {
		days := int(now.Sub(*a.DueAt).Hours() / 24)
		r.notifier.Notify(a.HolderID, Message{
			Title: fmt.Sprintf("设备已逾期未归还（超期 %d 天）", days),
			Content: fmt.Sprintf("您领用的「%s（资产编号 %s）」已于 %s 到期，目前逾期 %d 天。请尽快归还或提交续借申请，逾期记录将计入个人资产信用。",
				a.Name, a.AssetNo, a.DueAt.Format("2006年01月02日"), days),
			Type: "overdue", Level: "danger", RefType: "asset", RefID: a.ID,
		}, true)
		r.db.Model(&model.Asset{}).Where("id = ?", a.ID).Update("overdue_at", now)
		res.Notified++
	}

	// 管理员收到的是聚合摘要，而不是每台设备一条，避免通知爆炸。
	if len(dueSoonList)+len(overdueList) > 0 {
		r.notifier.NotifyRole([]string{"admin", "asset_manager"}, Message{
			Title: "设备到期提醒汇总",
			Content: fmt.Sprintf("本次扫描发现 %d 台设备即将到期、%d 台设备已逾期未归还，其中本次新推送提醒 %d 条。请前往「设备台账」筛选「已逾期」进行跟进。",
				res.DueSoon, res.Overdue, res.Notified),
			Type: "due", Level: "warning",
		})
		res.AdminDigest = true
	}
	return res, nil
}
