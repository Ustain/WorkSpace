package model

import "time"

// ---------- 枚举 ----------

// 资产状态机：
//
//	in_stock --借用审批通过--> in_use --归还审批通过--> in_stock
//	in_use/in_stock --报修--> repairing --修复--> in_stock
//	任意状态 --报废/遗失--> scrapped / lost（终态）
const (
	AssetInStock   = "in_stock"  // 在库可领用
	AssetInUse     = "in_use"    // 领用中
	AssetRepairing = "repairing" // 维修中
	AssetScrapped  = "scrapped"  // 已报废
	AssetLost      = "lost"      // 遗失
)

const (
	ApplyBorrow = "borrow" // 领用
	ApplyReturn = "return" // 归还
	ApplyRenew  = "renew"  // 续借
	ApplyScrap  = "scrap"  // 报废
)

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

const (
	RepairOpen       = "open"
	RepairProcessing = "processing"
	RepairResolved   = "resolved"
	RepairClosed     = "closed"
)

// 权限点，角色通过持有权限点集合获得能力。
const (
	PermAssetView     = "asset:view"
	PermAssetEdit     = "asset:edit"
	PermAssetDelete   = "asset:delete"
	PermAssetImport   = "asset:import"
	PermAssetExport   = "asset:export"
	PermApplyCreate   = "apply:create"
	PermApplyViewAll  = "apply:view_all"
	PermApplyApprove  = "apply:approve"
	PermRepairHandle  = "repair:handle"
	PermStatView      = "stat:view"
	PermUserManage    = "user:manage"
	PermRoleManage    = "role:manage"
	PermDeptManage    = "dept:manage"
	PermSettingManage = "setting:manage"
	PermAuditView     = "audit:view"
)

// AllPermissions 供角色管理页面渲染权限勾选树。
var AllPermissions = []struct {
	Group string `json:"group"`
	Code  string `json:"code"`
	Name  string `json:"name"`
}{
	{"资产", PermAssetView, "查看设备"},
	{"资产", PermAssetEdit, "新增/编辑设备"},
	{"资产", PermAssetDelete, "删除设备"},
	{"资产", PermAssetImport, "导入设备"},
	{"资产", PermAssetExport, "导出设备"},
	{"流程", PermApplyCreate, "发起申请"},
	{"流程", PermApplyViewAll, "查看全部申请"},
	{"流程", PermApplyApprove, "审批申请"},
	{"流程", PermRepairHandle, "处理报修工单"},
	{"系统", PermStatView, "查看统计看板"},
	{"系统", PermUserManage, "用户管理"},
	{"系统", PermRoleManage, "角色管理"},
	{"系统", PermDeptManage, "部门管理"},
	{"系统", PermSettingManage, "系统设置"},
	{"系统", PermAuditView, "审计日志"},
}

// ---------- 实体 ----------

type Department struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex" json:"name"`
	Code      string    `gorm:"size:32" json:"code"`
	Leader    string    `gorm:"size:32" json:"leader"`
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Role struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"size:32;uniqueIndex" json:"code"`
	Name        string    `gorm:"size:32" json:"name"`
	Description string    `gorm:"size:255" json:"description"`
	Permissions JSONSlice `gorm:"type:text" json:"permissions"`
	IsSystem    bool      `json:"isSystem"` // 系统内置角色禁止删除
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type User struct {
	ID           uint        `gorm:"primaryKey" json:"id"`
	Username     string      `gorm:"size:64;uniqueIndex" json:"username"`
	PasswordHash string      `gorm:"size:128" json:"-"`
	RealName     string      `gorm:"size:32" json:"realName"`
	Email        string      `gorm:"size:128" json:"email"`
	Phone        string      `gorm:"size:32" json:"phone"`
	DepartmentID uint        `json:"departmentId"`
	Department   *Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	RoleID       uint        `json:"roleId"`
	Role         *Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Enabled      bool        `gorm:"default:true" json:"enabled"`
	LastLoginAt  *time.Time  `json:"lastLoginAt"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

// Category 设备类型可由管理员维护，避免把"笔记本/显示器"等硬编码进代码。
type Category struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"size:32;uniqueIndex" json:"name"`
	Code            string    `gorm:"size:32" json:"code"`
	Icon            string    `gorm:"size:32" json:"icon"`
	DefaultLoanDays int       `gorm:"default:180" json:"defaultLoanDays"` // 该类型默认借用天数
	Sort            int       `json:"sort"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Asset struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	AssetNo       string     `gorm:"size:32;uniqueIndex" json:"assetNo"`
	Name          string     `gorm:"size:64;index" json:"name"`
	CategoryID    uint       `gorm:"index" json:"categoryId"`
	Category      *Category  `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Brand         string     `gorm:"size:32" json:"brand"`
	Model         string     `gorm:"size:64" json:"model"`
	SerialNo      string     `gorm:"size:64;index" json:"serialNo"`
	Specification string     `gorm:"size:255" json:"specification"`
	Status        string     `gorm:"size:16;index" json:"status"`
	Location      string     `gorm:"size:64" json:"location"`
	Price         float64    `json:"price"`
	PurchaseDate  *time.Time `json:"purchaseDate"`
	WarrantyUntil *time.Time `json:"warrantyUntil"`

	HolderID     uint        `gorm:"index" json:"holderId"` // 当前领用人
	Holder       *User       `gorm:"foreignKey:HolderID" json:"holder,omitempty"`
	HolderDeptID uint        `json:"holderDeptId"`
	HolderDept   *Department `gorm:"foreignKey:HolderDeptID" json:"holderDept,omitempty"`
	BorrowedAt   *time.Time  `gorm:"index" json:"borrowedAt"` // 领用时间
	DueAt        *time.Time  `gorm:"index" json:"dueAt"`      // 归还到期时间

	// 提醒去重标记：同一个借用周期内，提醒/超期各只推送一次。
	RemindedAt *time.Time `json:"-"`
	OverdueAt  *time.Time `json:"-"`

	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AssetRecord 资产履历，记录设备全生命周期的每一次状态变更。
type AssetRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AssetID    uint      `gorm:"index" json:"assetId"`
	Action     string    `gorm:"size:32" json:"action"`
	Operator   string    `gorm:"size:32" json:"operator"`
	OperatorID uint      `json:"operatorId"`
	Detail     string    `gorm:"size:512" json:"detail"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Application struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	ApplyNo     string     `gorm:"size:32;uniqueIndex" json:"applyNo"`
	Type        string     `gorm:"size:16;index" json:"type"`
	AssetID     uint       `gorm:"index" json:"assetId"`
	Asset       *Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	ApplicantID uint       `gorm:"index" json:"applicantId"`
	Applicant   *User      `gorm:"foreignKey:ApplicantID" json:"applicant,omitempty"`
	DeptID      uint       `json:"deptId"`
	Reason      string     `gorm:"size:255" json:"reason"`
	LoanDays    int        `json:"loanDays"`
	ExpectDueAt *time.Time `json:"expectDueAt"`

	Status         string     `gorm:"size:16;index" json:"status"`
	ApproverID     uint       `json:"approverId"`
	Approver       *User      `gorm:"foreignKey:ApproverID" json:"approver,omitempty"`
	ApproveComment string     `gorm:"size:255" json:"approveComment"`
	ApprovedAt     *time.Time `json:"approvedAt"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RepairTicket struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	TicketNo   string     `gorm:"size:32;uniqueIndex" json:"ticketNo"`
	AssetID    uint       `gorm:"index" json:"assetId"`
	Asset      *Asset     `gorm:"foreignKey:AssetID" json:"asset,omitempty"`
	ReporterID uint       `gorm:"index" json:"reporterId"`
	Reporter   *User      `gorm:"foreignKey:ReporterID" json:"reporter,omitempty"`
	Fault      string     `gorm:"size:512" json:"fault"`
	Status     string     `gorm:"size:16;index" json:"status"`
	HandlerID  uint       `json:"handlerId"`
	Handler    *User      `gorm:"foreignKey:HandlerID" json:"handler,omitempty"`
	Solution   string     `gorm:"size:512" json:"solution"`
	Cost       float64    `json:"cost"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"userId"`
	Title     string    `gorm:"size:128" json:"title"`
	Content   string    `gorm:"size:1024" json:"content"`
	Type      string    `gorm:"size:32;index" json:"type"` // approval / due / overdue / repair / system
	Level     string    `gorm:"size:16" json:"level"`      // info / warning / danger / success
	RefType   string    `gorm:"size:32" json:"refType"`    // asset / application / repair
	RefID     uint      `json:"refId"`
	IsRead    bool      `gorm:"index" json:"isRead"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

// EmailLog 邮件投递记录。真实发送与模拟发送都会落库，演示时可直接查看内容。
type EmailLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	To        string    `gorm:"size:128;index" json:"to"`
	Subject   string    `gorm:"size:255" json:"subject"`
	Body      string    `gorm:"type:text" json:"body"`
	Status    string    `gorm:"size:16" json:"status"` // sent / mocked / failed
	Error     string    `gorm:"size:255" json:"error"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"userId"`
	Username  string    `gorm:"size:64" json:"username"`
	Module    string    `gorm:"size:32;index" json:"module"`
	Action    string    `gorm:"size:64" json:"action"`
	Target    string    `gorm:"size:128" json:"target"`
	Detail    string    `gorm:"size:1024" json:"detail"`
	IP        string    `gorm:"size:64" json:"ip"`
	Method    string    `gorm:"size:8" json:"method"`
	Path      string    `gorm:"size:128" json:"path"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Models 供 AutoMigrate 统一注册。
func Models() []any {
	return []any{
		&Department{}, &Role{}, &User{}, &Category{}, &Asset{}, &AssetRecord{},
		&Application{}, &RepairTicket{}, &Notification{}, &EmailLog{}, &AuditLog{}, &Setting{},
	}
}
