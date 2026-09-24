package service

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"ams/internal/model"
)

// Workflow 承载领用/归还/续借/报废四类申请的提交与审批。
//
// 设计要点：
//   - 提交时不动设备状态，只做可行性校验；真正改状态发生在审批通过的事务里，
//     避免"申请一堆但都没批"把设备占死。
//   - 审批通过时二次校验设备状态，防止两个人同时申请同一台设备造成超领。
type Workflow struct {
	db       *gorm.DB
	notifier *Notifier
	settings *SettingService
}

func NewWorkflow(db *gorm.DB, n *Notifier, s *SettingService) *Workflow {
	return &Workflow{db: db, notifier: n, settings: s}
}

var (
	ErrAssetNotFound   = errors.New("设备不存在")
	ErrAssetNotUsable  = errors.New("该设备当前不可领用")
	ErrNotHolder       = errors.New("您不是该设备的领用人")
	ErrDuplicateApply  = errors.New("该设备已有待审批的申请，请勿重复提交")
	ErrApplyNotPending = errors.New("该申请已被处理，无需重复操作")
	ErrLoanDaysInvalid = errors.New("借用天数超出系统允许范围")
)

type SubmitInput struct {
	Type      string
	AssetID   uint
	Reason    string
	LoanDays  int
	Applicant *model.User
}

// Submit 创建一条申请并通知审批人。
func (w *Workflow) Submit(in SubmitInput) (*model.Application, error) {
	var asset model.Asset
	if err := w.db.Preload("Category").First(&asset, in.AssetID).Error; err != nil {
		return nil, ErrAssetNotFound
	}

	// 同一设备同一类型只允许存在一条待审批申请。
	var dup int64
	w.db.Model(&model.Application{}).
		Where("asset_id = ? AND type = ? AND status = ?", in.AssetID, in.Type, model.StatusPending).
		Count(&dup)
	if dup > 0 {
		return nil, ErrDuplicateApply
	}

	switch in.Type {
	case model.ApplyBorrow:
		if asset.Status != model.AssetInStock {
			return nil, fmt.Errorf("%w（当前状态：%s）", ErrAssetNotUsable, StatusLabel(asset.Status))
		}
		if in.LoanDays <= 0 {
			in.LoanDays = defaultLoanDays(asset.Category)
		}
		maxDays := w.settings.Int("max_loan_days", 365)
		if in.LoanDays > maxDays {
			return nil, fmt.Errorf("%w（最长 %d 天）", ErrLoanDaysInvalid, maxDays)
		}
	case model.ApplyReturn, model.ApplyRenew:
		if asset.Status != model.AssetInUse {
			return nil, fmt.Errorf("%w（当前状态：%s）", ErrAssetNotUsable, StatusLabel(asset.Status))
		}
		if asset.HolderID != in.Applicant.ID {
			return nil, ErrNotHolder
		}
		if in.Type == model.ApplyRenew && in.LoanDays <= 0 {
			in.LoanDays = 90
		}
	case model.ApplyScrap:
		if asset.Status == model.AssetScrapped {
			return nil, errors.New("该设备已报废")
		}
	default:
		return nil, errors.New("不支持的申请类型")
	}

	now := time.Now()
	app := model.Application{
		ApplyNo:     genApplyNo(w.db, in.Type, now),
		Type:        in.Type,
		AssetID:     asset.ID,
		ApplicantID: in.Applicant.ID,
		DeptID:      in.Applicant.DepartmentID,
		Reason:      in.Reason,
		LoanDays:    in.LoanDays,
		Status:      model.StatusPending,
	}
	if in.LoanDays > 0 {
		base := now
		// 续借在原到期日基础上顺延，而不是从今天重新起算。
		if in.Type == model.ApplyRenew && asset.DueAt != nil && asset.DueAt.After(now) {
			base = *asset.DueAt
		}
		app.ExpectDueAt = ptrTime(base.AddDate(0, 0, in.LoanDays))
	}
	if err := w.db.Create(&app).Error; err != nil {
		return nil, err
	}

	Record(w.db, asset.ID, "apply", in.Applicant,
		fmt.Sprintf("提交%s申请 %s，事由：%s", ApplyTypeLabels[in.Type], app.ApplyNo, in.Reason))

	w.notifier.NotifyRole([]string{"admin", "asset_manager"}, Message{
		Title: fmt.Sprintf("新的%s申请待审批", ApplyTypeLabels[in.Type]),
		Content: fmt.Sprintf("%s 提交了「%s（%s）」的%s申请，单号 %s，事由：%s。请前往审批中心处理。",
			in.Applicant.RealName, asset.Name, asset.AssetNo, ApplyTypeLabels[in.Type], app.ApplyNo, in.Reason),
		Type: "approval", Level: "info", RefType: "application", RefID: app.ID,
	})
	return &app, nil
}

// Approve 审批通过：在一个事务里改申请状态 + 改设备状态，两者必须同生共死。
func (w *Workflow) Approve(appID uint, approver *model.User, comment string) (*model.Application, error) {
	var app model.Application
	err := w.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("Asset").Preload("Applicant").First(&app, appID).Error; err != nil {
			return ErrApplyNotPending
		}
		if app.Status != model.StatusPending {
			return ErrApplyNotPending
		}
		asset := app.Asset
		if asset == nil {
			return ErrAssetNotFound
		}

		now := time.Now()
		updates := map[string]any{}
		switch app.Type {
		case model.ApplyBorrow:
			// 二次校验，挡住并发审批导致的超领。
			if asset.Status != model.AssetInStock {
				return fmt.Errorf("%w（当前状态：%s）", ErrAssetNotUsable, StatusLabel(asset.Status))
			}
			due := now.AddDate(0, 0, app.LoanDays)
			if app.ExpectDueAt != nil {
				due = *app.ExpectDueAt
			}
			updates = map[string]any{
				"status": model.AssetInUse, "holder_id": app.ApplicantID,
				"holder_dept_id": app.DeptID, "borrowed_at": now, "due_at": due,
				"reminded_at": nil, "overdue_at": nil,
			}
		case model.ApplyReturn:
			updates = map[string]any{
				"status": model.AssetInStock, "holder_id": 0, "holder_dept_id": 0,
				"borrowed_at": nil, "due_at": nil, "reminded_at": nil, "overdue_at": nil,
			}
		case model.ApplyRenew:
			due := now.AddDate(0, 0, app.LoanDays)
			if app.ExpectDueAt != nil {
				due = *app.ExpectDueAt
			}
			// 续借后重置提醒标记，新周期到期前会重新提醒。
			updates = map[string]any{"due_at": due, "reminded_at": nil, "overdue_at": nil}
		case model.ApplyScrap:
			updates = map[string]any{
				"status": model.AssetScrapped, "holder_id": 0, "holder_dept_id": 0,
				"borrowed_at": nil, "due_at": nil, "reminded_at": nil, "overdue_at": nil,
			}
		}
		if err := tx.Model(&model.Asset{}).Where("id = ?", asset.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&app).Updates(map[string]any{
			"status": model.StatusApproved, "approver_id": approver.ID,
			"approve_comment": comment, "approved_at": now,
		}).Error
	})
	if err != nil {
		return nil, err
	}

	Record(w.db, app.AssetID, app.Type, approver,
		fmt.Sprintf("%s申请 %s 审批通过。%s", ApplyTypeLabels[app.Type], app.ApplyNo, comment))

	detail := ""
	if app.Type == model.ApplyBorrow || app.Type == model.ApplyRenew {
		var fresh model.Asset
		if w.db.First(&fresh, app.AssetID).Error == nil && fresh.DueAt != nil {
			detail = fmt.Sprintf("应归还日期为 %s，系统会在到期前提醒您。", fresh.DueAt.Format("2006年01月02日"))
		}
	}
	w.notifier.Notify(app.ApplicantID, Message{
		Title: fmt.Sprintf("%s申请已通过", ApplyTypeLabels[app.Type]),
		Content: fmt.Sprintf("您的%s申请（单号 %s，设备「%s」）已由 %s 审批通过。%s 审批意见：%s",
			ApplyTypeLabels[app.Type], app.ApplyNo, app.Asset.Name, approver.RealName, detail, orDash(comment)),
		Type: "approval", Level: "success", RefType: "application", RefID: app.ID,
	}, true)
	return &app, nil
}

// Reject 驳回申请，不触碰设备状态。
func (w *Workflow) Reject(appID uint, approver *model.User, comment string) (*model.Application, error) {
	var app model.Application
	if err := w.db.Preload("Asset").First(&app, appID).Error; err != nil {
		return nil, ErrApplyNotPending
	}
	if app.Status != model.StatusPending {
		return nil, ErrApplyNotPending
	}
	now := time.Now()
	if err := w.db.Model(&app).Updates(map[string]any{
		"status": model.StatusRejected, "approver_id": approver.ID,
		"approve_comment": comment, "approved_at": now,
	}).Error; err != nil {
		return nil, err
	}
	Record(w.db, app.AssetID, "reject", approver,
		fmt.Sprintf("%s申请 %s 被驳回。理由：%s", ApplyTypeLabels[app.Type], app.ApplyNo, comment))
	w.notifier.Notify(app.ApplicantID, Message{
		Title: fmt.Sprintf("%s申请被驳回", ApplyTypeLabels[app.Type]),
		Content: fmt.Sprintf("您的%s申请（单号 %s，设备「%s」）未通过审批。驳回理由：%s",
			ApplyTypeLabels[app.Type], app.ApplyNo, app.Asset.Name, orDash(comment)),
		Type: "approval", Level: "danger", RefType: "application", RefID: app.ID,
	}, true)
	return &app, nil
}

// Cancel 申请人主动撤回自己的待审批申请。
func (w *Workflow) Cancel(appID uint, operator *model.User) error {
	var app model.Application
	if err := w.db.First(&app, appID).Error; err != nil {
		return ErrApplyNotPending
	}
	if app.Status != model.StatusPending {
		return ErrApplyNotPending
	}
	if app.ApplicantID != operator.ID {
		return errors.New("只能撤销自己提交的申请")
	}
	if err := w.db.Model(&app).Update("status", model.StatusCancelled).Error; err != nil {
		return err
	}
	Record(w.db, app.AssetID, "cancel", operator, fmt.Sprintf("申请 %s 已由申请人撤销", app.ApplyNo))
	return nil
}

func defaultLoanDays(c *model.Category) int {
	if c != nil && c.DefaultLoanDays > 0 {
		return c.DefaultLoanDays
	}
	return 180
}

func genApplyNo(db *gorm.DB, typ string, t time.Time) string {
	prefix := map[string]string{
		model.ApplyBorrow: "BR", model.ApplyReturn: "RT",
		model.ApplyRenew: "RN", model.ApplyScrap: "SC",
	}[typ]
	if prefix == "" {
		prefix = "AP"
	}
	day := t.Format("20060102")
	var n int64
	db.Model(&model.Application{}).Where("apply_no LIKE ?", prefix+day+"%").Count(&n)
	return fmt.Sprintf("%s%s%04d", prefix, day, n+1)
}

func ptrTime(t time.Time) *time.Time { return &t }

func orDash(s string) string {
	if s == "" {
		return "（无）"
	}
	return s
}
