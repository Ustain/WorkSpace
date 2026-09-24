package store

import (
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"

	"ams/internal/auth"
	"ams/internal/model"
)

// DefaultSettings 首次启动写入的系统参数默认值。
var DefaultSettings = map[string]string{
	"system_name":        "科研机构 IT 资产管理系统",
	"org_name":           "国家先进计算技术研究院",
	"remind_before_days": "7",    // 到期前多少天开始提醒
	"max_loan_days":      "365",  // 单次领用最长天数
	"email_notify":       "true", // 普通用户是否额外发邮件
	"auto_approve_small": "false",
}

// Seed 仅在库为空时灌入演示数据，重复启动不会污染已有数据。
func Seed(db *gorm.DB) error {
	var userCount int64
	if err := db.Model(&model.User{}).Count(&userCount).Error; err != nil {
		return err
	}
	if err := seedSettings(db); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		depts, err := seedDepartments(tx)
		if err != nil {
			return err
		}
		roles, err := seedRoles(tx)
		if err != nil {
			return err
		}
		users, err := seedUsers(tx, depts, roles)
		if err != nil {
			return err
		}
		cats, err := seedCategories(tx)
		if err != nil {
			return err
		}
		assets, err := seedAssets(tx, cats, users)
		if err != nil {
			return err
		}
		if err := seedApplications(tx, assets, users); err != nil {
			return err
		}
		if err := seedRepairs(tx, assets, users); err != nil {
			return err
		}
		return seedNotifications(tx, users)
	})
}

func seedSettings(db *gorm.DB) error {
	for k, v := range DefaultSettings {
		var n int64
		if err := db.Model(&model.Setting{}).Where("key = ?", k).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			if err := db.Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedDepartments(tx *gorm.DB) ([]model.Department, error) {
	depts := []model.Department{
		{Name: "高性能计算研究所", Code: "HPC", Leader: "周衡", Remark: "承担超算与并行算法方向"},
		{Name: "人工智能研究中心", Code: "AI", Leader: "林岚", Remark: "大模型与多模态方向"},
		{Name: "材料科学实验室", Code: "MAT", Leader: "赵启明", Remark: "新型半导体材料"},
		{Name: "生命科学实验室", Code: "BIO", Leader: "孙雅文", Remark: "基因组学与生信"},
		{Name: "信息中心", Code: "IT", Leader: "陈默", Remark: "全院 IT 基础设施与资产归口"},
		{Name: "综合办公室", Code: "ADM", Leader: "吴静", Remark: "行政与后勤保障"},
	}
	if err := tx.Create(&depts).Error; err != nil {
		return nil, err
	}
	return depts, nil
}

func seedRoles(tx *gorm.DB) (map[string]model.Role, error) {
	all := make(model.JSONSlice, 0, len(model.AllPermissions))
	for _, p := range model.AllPermissions {
		all = append(all, p.Code)
	}
	roles := []model.Role{
		{
			Code: "admin", Name: "系统管理员", IsSystem: true,
			Description: "拥有全部权限，负责资产台账、审批与系统配置",
			Permissions: all,
		},
		{
			Code: "asset_manager", Name: "资产管理员", IsSystem: true,
			Description: "负责设备台账维护与领用审批，不涉及用户与角色管理",
			Permissions: model.JSONSlice{
				model.PermAssetView, model.PermAssetEdit, model.PermAssetImport, model.PermAssetExport,
				model.PermApplyCreate, model.PermApplyViewAll, model.PermApplyApprove,
				model.PermRepairHandle, model.PermStatView,
			},
		},
		{
			Code: "user", Name: "普通用户", IsSystem: true,
			Description: "可查询可用设备、发起领用/归还/续借申请、提交报修",
			Permissions: model.JSONSlice{model.PermAssetView, model.PermApplyCreate},
		},
	}
	if err := tx.Create(&roles).Error; err != nil {
		return nil, err
	}
	m := map[string]model.Role{}
	for _, r := range roles {
		m[r.Code] = r
	}
	return m, nil
}

func seedUsers(tx *gorm.DB, depts []model.Department, roles map[string]model.Role) ([]model.User, error) {
	hash, err := auth.HashPassword("admin123")
	if err != nil {
		return nil, err
	}
	userHash, err := auth.HashPassword("user123")
	if err != nil {
		return nil, err
	}

	type spec struct {
		username, realName, email, phone, role string
		dept                                   int
	}
	specs := []spec{
		{"admin", "陈默", "chenmo@institute.ac.cn", "13800000001", "admin", 4},
		{"assetmgr", "吴静", "wujing@institute.ac.cn", "13800000002", "asset_manager", 5},
		{"zhouheng", "周衡", "zhouheng@institute.ac.cn", "13800000010", "user", 0},
		{"lilei", "李磊", "lilei@institute.ac.cn", "13800000011", "user", 0},
		{"linlan", "林岚", "linlan@institute.ac.cn", "13800000012", "user", 1},
		{"wangyu", "王宇", "wangyu@institute.ac.cn", "13800000013", "user", 1},
		{"hanmeimei", "韩梅梅", "hanmm@institute.ac.cn", "13800000014", "user", 1},
		{"zhaoqiming", "赵启明", "zhaoqm@institute.ac.cn", "13800000015", "user", 2},
		{"qianduo", "钱多", "qianduo@institute.ac.cn", "13800000016", "user", 2},
		{"sunyawen", "孙雅文", "sunyw@institute.ac.cn", "13800000017", "user", 3},
		{"zhengkai", "郑凯", "zhengkai@institute.ac.cn", "13800000018", "user", 3},
		{"fengxiao", "冯晓", "fengxiao@institute.ac.cn", "13800000019", "user", 5},
	}

	users := make([]model.User, 0, len(specs))
	for _, s := range specs {
		pwd := userHash
		if s.role != "user" {
			pwd = hash
		}
		users = append(users, model.User{
			Username: s.username, PasswordHash: pwd, RealName: s.realName,
			Email: s.email, Phone: s.phone,
			DepartmentID: depts[s.dept].ID, RoleID: roles[s.role].ID, Enabled: true,
		})
	}
	if err := tx.Create(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func seedCategories(tx *gorm.DB) ([]model.Category, error) {
	cats := []model.Category{
		{Name: "笔记本电脑", Code: "NB", Icon: "Monitor", DefaultLoanDays: 365, Sort: 1},
		{Name: "台式机", Code: "PC", Icon: "Platform", DefaultLoanDays: 365, Sort: 2},
		{Name: "显示器", Code: "MN", Icon: "FullScreen", DefaultLoanDays: 365, Sort: 3},
		{Name: "鼠标", Code: "MS", Icon: "Mouse", DefaultLoanDays: 180, Sort: 4},
		{Name: "键盘", Code: "KB", Icon: "Operation", DefaultLoanDays: 180, Sort: 5},
		{Name: "转接头", Code: "AD", Icon: "Connection", DefaultLoanDays: 90, Sort: 6},
		{Name: "投影仪", Code: "PJ", Icon: "VideoCamera", DefaultLoanDays: 30, Sort: 7},
		{Name: "打印机", Code: "PR", Icon: "Printer", DefaultLoanDays: 365, Sort: 8},
		{Name: "移动硬盘", Code: "HD", Icon: "Coin", DefaultLoanDays: 90, Sort: 9},
		{Name: "服务器", Code: "SV", Icon: "Cpu", DefaultLoanDays: 730, Sort: 10},
		{Name: "平板电脑", Code: "TB", Icon: "Iphone", DefaultLoanDays: 180, Sort: 11},
		{Name: "耳机", Code: "HP", Icon: "Headset", DefaultLoanDays: 180, Sort: 12},
	}
	if err := tx.Create(&cats).Error; err != nil {
		return nil, err
	}
	return cats, nil
}

type assetTpl struct {
	catIdx int
	name   string
	brand  string
	model  string
	spec   string
	price  float64
	count  int
}

func seedAssets(tx *gorm.DB, cats []model.Category, users []model.User) ([]model.Asset, error) {
	// 固定随机种子，保证每次初始化得到同一份演示数据，便于录屏和对照讲解。
	rnd := rand.New(rand.NewSource(20240918))
	now := time.Now()

	tpls := []assetTpl{
		{0, "ThinkPad X1 Carbon", "Lenovo", "X1C Gen11", "i7-1360P / 32G / 1T SSD / 14英寸", 12800, 12},
		{0, "MacBook Pro 14", "Apple", "M3 Pro", "M3 Pro / 36G / 1T SSD", 19999, 6},
		{0, "戴尔 Latitude 5440", "Dell", "L5440", "i5-1345U / 16G / 512G SSD", 7600, 8},
		{1, "联想 ThinkCentre M930t", "Lenovo", "M930t", "i7-13700 / 32G / 1T SSD", 8200, 8},
		{1, "工作站 Precision 3660", "Dell", "P3660", "i9-13900K / 64G / RTX A4000", 26800, 4},
		{2, "戴尔 27 寸 4K 显示器", "Dell", "U2723QE", "27英寸 / 3840x2160 / IPS", 3999, 14},
		{2, "AOC 24 寸办公显示器", "AOC", "24B2XH", "23.8英寸 / 1920x1080", 799, 10},
		{3, "罗technic 无线鼠标", "Logitech", "MX Master 3S", "蓝牙+2.4G / 8000DPI", 699, 16},
		{3, "雷柏有线鼠标", "Rapoo", "N100", "USB 有线 / 1000DPI", 39, 20},
		{4, "罗技机械键盘", "Logitech", "MX Mechanical", "104键 / 茶轴 / 无线", 899, 12},
		{4, "联想有线键盘", "Lenovo", "KB-1021", "104键 / USB", 89, 18},
		{5, "Type-C 多功能扩展坞", "Anker", "PowerExpand 8-in-1", "HDMI+USB3.0x3+RJ45+PD100W", 459, 16},
		{5, "Mini DP 转 HDMI", "UGREEN", "MD112", "4K@30Hz", 69, 12},
		{6, "爱普生激光投影仪", "Epson", "CB-FH52", "4000流明 / 1080P", 6999, 5},
		{6, "明基便携投影仪", "BenQ", "GV31", "300流明 / 便携", 3299, 3},
		{7, "惠普黑白激光打印机", "HP", "M405dn", "A4 / 双面 / 网络", 2399, 4},
		{8, "西部数据移动硬盘 4T", "WD", "Elements 4TB", "USB3.0 / 4TB", 699, 10},
		{9, "浪潮机架服务器", "Inspur", "NF5280M6", "2x Xeon 6330 / 256G / 8x4T", 89000, 4},
		{10, "iPad Pro 11", "Apple", "M4 11寸", "256G / WiFi", 7999, 4},
		{11, "降噪会议耳机", "Jabra", "Evolve2 65", "蓝牙 / 主动降噪", 1299, 8},
	}

	locations := []string{"A座3层机房", "A座5层办公区", "B座2层实验室", "B座4层会议室", "C座1层仓库", "C座6层办公区"}

	// 只让真人用户（非管理账号）出现在领用人里，更贴近真实场景。
	borrowers := users[2:]

	assets := make([]model.Asset, 0, 200)
	seq := map[string]int{}
	for _, t := range tpls {
		cat := cats[t.catIdx]
		for i := 0; i < t.count; i++ {
			seq[cat.Code]++
			purchase := now.AddDate(0, -rnd.Intn(48)-1, -rnd.Intn(28))
			warranty := purchase.AddDate(rnd.Intn(3)+1, 0, 0)
			a := model.Asset{
				AssetNo:       fmt.Sprintf("%s-%s-%04d", cat.Code, purchase.Format("2006"), seq[cat.Code]),
				Name:          t.name,
				CategoryID:    cat.ID,
				Brand:         t.brand,
				Model:         t.model,
				SerialNo:      fmt.Sprintf("SN%s%06d", cat.Code, rnd.Intn(999999)),
				Specification: t.spec,
				Location:      locations[rnd.Intn(len(locations))],
				Price:         t.price,
				PurchaseDate:  ptr(purchase),
				WarrantyUntil: ptr(warranty),
				Status:        model.AssetInStock,
			}
			assets = append(assets, a)
		}
	}

	// 按比例铺开状态分布，并刻意构造"即将到期/已超期"的样本用于演示提醒功能。
	for i := range assets {
		switch r := rnd.Float64(); {
		case r < 0.52: // 领用中
			u := borrowers[rnd.Intn(len(borrowers))]
			borrowed := now.AddDate(0, 0, -rnd.Intn(300)-10)
			var due time.Time
			switch {
			case i%17 == 0: // 已超期
				due = now.AddDate(0, 0, -rnd.Intn(20)-1)
			case i%11 == 0: // 7 天内到期
				due = now.AddDate(0, 0, rnd.Intn(7))
			default:
				due = borrowed.AddDate(0, 0, 180+rnd.Intn(365))
			}
			assets[i].Status = model.AssetInUse
			assets[i].HolderID = u.ID
			assets[i].HolderDeptID = u.DepartmentID
			assets[i].BorrowedAt = ptr(borrowed)
			assets[i].DueAt = ptr(due)
		case r < 0.60:
			assets[i].Status = model.AssetRepairing
			assets[i].Remark = "送修中，等待厂商返修"
		case r < 0.65:
			assets[i].Status = model.AssetScrapped
			assets[i].Remark = "超过使用年限，已走报废流程"
		case r < 0.665:
			assets[i].Status = model.AssetLost
			assets[i].Remark = "盘点时未找到，待核实"
		default:
			assets[i].Status = model.AssetInStock
		}
	}

	if err := tx.Create(&assets).Error; err != nil {
		return nil, err
	}

	// 为每台设备写入入库履历，领用中的补一条领用履历。
	records := make([]model.AssetRecord, 0, len(assets)*2)
	for _, a := range assets {
		records = append(records, model.AssetRecord{
			AssetID: a.ID, Action: "created", Operator: "系统初始化",
			Detail:    fmt.Sprintf("设备入库，采购价 ¥%.2f，存放于 %s", a.Price, a.Location),
			CreatedAt: derefOr(a.PurchaseDate, now),
		})
		if a.Status == model.AssetInUse && a.BorrowedAt != nil {
			holder := "—"
			for _, u := range users {
				if u.ID == a.HolderID {
					holder = u.RealName
				}
			}
			records = append(records, model.AssetRecord{
				AssetID: a.ID, Action: "borrow", Operator: holder, OperatorID: a.HolderID,
				Detail:    fmt.Sprintf("领用人 %s，应归还日期 %s", holder, a.DueAt.Format("2006-01-02")),
				CreatedAt: *a.BorrowedAt,
			})
		}
	}
	if err := tx.CreateInBatches(&records, 200).Error; err != nil {
		return nil, err
	}
	return assets, nil
}

func seedApplications(tx *gorm.DB, assets []model.Asset, users []model.User) error {
	rnd := rand.New(rand.NewSource(88))
	now := time.Now()
	admin := users[0]

	var inStock, inUse []model.Asset
	for _, a := range assets {
		switch a.Status {
		case model.AssetInStock:
			inStock = append(inStock, a)
		case model.AssetInUse:
			inUse = append(inUse, a)
		}
	}

	apps := make([]model.Application, 0, 40)
	reasons := []string{
		"新入职科研助理配备办公设备", "课题组数据处理需要高性能机器", "实验室学术汇报需要投影设备",
		"原设备故障返修，临时替换使用", "外出参加学术会议携带使用", "新建工位配置外设",
		"多屏调试需要额外显示器", "数据备份归档需要移动存储",
	}

	// 待审批：管理员登录后首页就能看到待办。
	for i := 0; i < 6 && i < len(inStock); i++ {
		u := users[2+rnd.Intn(len(users)-2)]
		created := now.AddDate(0, 0, -rnd.Intn(5))
		days := 90 + rnd.Intn(275)
		apps = append(apps, model.Application{
			ApplyNo: applyNo(model.ApplyBorrow, created, i+1), Type: model.ApplyBorrow,
			AssetID: inStock[i].ID, ApplicantID: u.ID, DeptID: u.DepartmentID,
			Reason: reasons[rnd.Intn(len(reasons))], LoanDays: days,
			ExpectDueAt: ptr(created.AddDate(0, 0, days)),
			Status:      model.StatusPending, CreatedAt: created, UpdatedAt: created,
		})
	}
	// 历史已审批 / 驳回 / 归还 / 续借，凑出有说服力的流程记录。
	for i := 0; i < 24 && i < len(inUse); i++ {
		u := users[2+rnd.Intn(len(users)-2)]
		created := now.AddDate(0, 0, -rnd.Intn(240)-6)
		approved := created.Add(time.Duration(rnd.Intn(48)+2) * time.Hour)
		status := model.StatusApproved
		comment := "同意领用，请按期归还并妥善保管。"
		typ := model.ApplyBorrow
		switch {
		case i%7 == 3:
			status, comment = model.StatusRejected, "库存紧张，建议先复用本组闲置设备。"
		case i%7 == 5:
			typ, comment = model.ApplyRenew, "同意续借，请于新到期日前归还。"
		case i%7 == 6:
			typ, comment = model.ApplyReturn, "已核验设备外观与配件，确认归还。"
		}
		days := 90 + rnd.Intn(275)
		apps = append(apps, model.Application{
			ApplyNo: applyNo(typ, created, 100+i), Type: typ,
			AssetID: inUse[i].ID, ApplicantID: u.ID, DeptID: u.DepartmentID,
			Reason: reasons[rnd.Intn(len(reasons))], LoanDays: days,
			ExpectDueAt: ptr(created.AddDate(0, 0, days)),
			Status:      status, ApproverID: admin.ID, ApproveComment: comment,
			ApprovedAt: ptr(approved), CreatedAt: created, UpdatedAt: approved,
		})
	}
	return tx.CreateInBatches(&apps, 100).Error
}

func seedRepairs(tx *gorm.DB, assets []model.Asset, users []model.User) error {
	rnd := rand.New(rand.NewSource(1234))
	now := time.Now()
	faults := []string{
		"屏幕出现竖条纹，疑似排线接触不良", "无法开机，电源指示灯不亮", "键盘部分按键失灵",
		"风扇异响且温度偏高", "投影画面偏色严重", "USB 接口接触不良，外设频繁掉线",
		"电池续航骤降，充电无法到 100%",
	}
	var repairing []model.Asset
	for _, a := range assets {
		if a.Status == model.AssetRepairing {
			repairing = append(repairing, a)
		}
	}
	tickets := make([]model.RepairTicket, 0, len(repairing)+6)
	for i, a := range repairing {
		u := users[2+rnd.Intn(len(users)-2)]
		created := now.AddDate(0, 0, -rnd.Intn(30)-1)
		st := model.RepairOpen
		if i%3 == 1 {
			st = model.RepairProcessing
		}
		tickets = append(tickets, model.RepairTicket{
			TicketNo: fmt.Sprintf("RP%s%04d", created.Format("20060102"), i+1),
			AssetID:  a.ID, ReporterID: u.ID, Fault: faults[rnd.Intn(len(faults))],
			Status: st, CreatedAt: created, UpdatedAt: created,
		})
	}
	// 几条已完结工单，让统计图有历史数据。
	for i := 0; i < 6 && i < len(assets); i++ {
		a := assets[i*13%len(assets)]
		u := users[2+rnd.Intn(len(users)-2)]
		created := now.AddDate(0, 0, -rnd.Intn(150)-30)
		resolved := created.AddDate(0, 0, rnd.Intn(10)+1)
		tickets = append(tickets, model.RepairTicket{
			TicketNo: fmt.Sprintf("RP%s%04d", created.Format("20060102"), 900+i),
			AssetID:  a.ID, ReporterID: u.ID, Fault: faults[rnd.Intn(len(faults))],
			Status: model.RepairResolved, HandlerID: users[1].ID,
			Solution: "更换故障配件后测试通过，已归还使用人。", Cost: float64(rnd.Intn(1500) + 80),
			ResolvedAt: ptr(resolved), CreatedAt: created, UpdatedAt: resolved,
		})
	}
	return tx.CreateInBatches(&tickets, 100).Error
}

func seedNotifications(tx *gorm.DB, users []model.User) error {
	now := time.Now()
	ns := []model.Notification{
		{UserID: users[0].ID, Title: "欢迎使用资产管理系统", Type: "system", Level: "info",
			Content:   "您当前以系统管理员身份登录，可在「审批中心」处理待办申请，在「统计看板」查看资产概览。",
			CreatedAt: now.Add(-2 * time.Hour)},
		{UserID: users[0].ID, Title: "季度资产盘点提醒", Type: "system", Level: "warning",
			Content:   "本季度资产盘点将于月底开始，请提前核对在库与领用中设备的实物一致性。",
			CreatedAt: now.Add(-30 * time.Minute)},
		{UserID: users[2].ID, Title: "欢迎使用资产管理系统", Type: "system", Level: "info",
			Content:   "您可在「可领用设备」中挑选设备发起领用申请，审批结果会通过站内信与邮件通知您。",
			CreatedAt: now.Add(-3 * time.Hour)},
	}
	return tx.Create(&ns).Error
}

// ---------- 小工具 ----------

func applyNo(typ string, t time.Time, seq int) string {
	prefix := map[string]string{
		model.ApplyBorrow: "BR", model.ApplyReturn: "RT",
		model.ApplyRenew: "RN", model.ApplyScrap: "SC",
	}[typ]
	if prefix == "" {
		prefix = "AP"
	}
	return fmt.Sprintf("%s%s%04d", prefix, t.Format("20060102"), seq)
}

func ptr[T any](v T) *T { return &v }

func derefOr(p *time.Time, def time.Time) time.Time {
	if p == nil {
		return def
	}
	return *p
}
