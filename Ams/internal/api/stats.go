package api

import (
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
	"ams/internal/service"
)

// statsOverview 首页顶部的数字卡片。
func (s *Server) statsOverview(c *gin.Context) {
	now := time.Now()
	days := s.settings.Int("remind_before_days", 7)

	countAssets := func(where string, args ...any) int64 {
		var n int64
		q := s.db.Model(&model.Asset{})
		if where != "" {
			q = q.Where(where, args...)
		}
		q.Count(&n)
		return n
	}

	var totalValue, inUseValue float64
	s.db.Model(&model.Asset{}).Where("status <> ?", model.AssetScrapped).
		Select("COALESCE(SUM(price),0)").Scan(&totalValue)
	s.db.Model(&model.Asset{}).Where("status = ?", model.AssetInUse).
		Select("COALESCE(SUM(price),0)").Scan(&inUseValue)

	total := countAssets("")
	inUse := countAssets("status = ?", model.AssetInUse)

	var pendingApply, openRepair, userCount int64
	s.db.Model(&model.Application{}).Where("status = ?", model.StatusPending).Count(&pendingApply)
	s.db.Model(&model.RepairTicket{}).
		Where("status IN ?", []string{model.RepairOpen, model.RepairProcessing}).Count(&openRepair)
	s.db.Model(&model.User{}).Where("enabled = ?", true).Count(&userCount)

	// 在用率 = 领用中 ÷（总数 − 已报废）
	usable := total - countAssets("status = ?", model.AssetScrapped)
	rate := 0.0
	if usable > 0 {
		rate = float64(inUse) / float64(usable) * 100
	}

	response.OK(c, gin.H{
		"totalAssets": total,
		"inUse":       inUse,
		"inStock":     countAssets("status = ?", model.AssetInStock),
		"repairing":   countAssets("status = ?", model.AssetRepairing),
		"scrapped":    countAssets("status = ?", model.AssetScrapped),
		"lost":        countAssets("status = ?", model.AssetLost),
		"overdue": countAssets("status = ? AND due_at IS NOT NULL AND due_at < ?",
			model.AssetInUse, now),
		"dueSoon": countAssets("status = ? AND due_at IS NOT NULL AND due_at >= ? AND due_at <= ?",
			model.AssetInUse, now, now.AddDate(0, 0, days)),
		"warrantyExpiring": countAssets("warranty_until IS NOT NULL AND warranty_until >= ? AND warranty_until <= ?",
			now, now.AddDate(0, 0, 90)),
		"totalValue":   round2(totalValue),
		"inUseValue":   round2(inUseValue),
		"pendingApply": pendingApply,
		"openRepair":   openRepair,
		"userCount":    userCount,
		"usageRate":    round2(rate),
		"remindDays":   days,
	})
}

type nameValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// nameCount 承接各类 GROUP BY 查询结果。
type nameCount struct {
	Name string
	N    float64
}

func toNV(rows []nameCount) []nameValue {
	out := make([]nameValue, 0, len(rows))
	for _, r := range rows {
		name := r.Name
		if name == "" {
			name = "未分类"
		}
		out = append(out, nameValue{Name: name, Value: round2(r.N)})
	}
	return out
}

// statsCharts 一次性返回首页所有图表的数据源，避免前端串行请求多次。
func (s *Server) statsCharts(c *gin.Context) {
	response.OK(c, gin.H{
		"byCategory":      s.chartByCategory(),
		"byStatus":        s.chartByStatus(),
		"byDept":          s.chartByDept(),
		"valueByCategory": s.chartValueByCategory(),
		"monthlyTrend":    s.chartMonthlyTrend(12),
		"purchaseByYear":  s.chartPurchaseByYear(),
		"dueForecast":     s.chartDueForecast(),
		"topHolders":      s.chartTopHolders(8),
	})
}

func (s *Server) chartByCategory() []nameValue {
	var rows []nameCount
	s.db.Model(&model.Asset{}).
		Select("categories.name AS name, COUNT(assets.id) AS n").
		Joins("LEFT JOIN categories ON categories.id = assets.category_id").
		Group("categories.name").Order("n DESC").Scan(&rows)
	return toNV(rows)
}

func (s *Server) chartValueByCategory() []nameValue {
	var rows []nameCount
	s.db.Model(&model.Asset{}).
		Select("categories.name AS name, COALESCE(SUM(assets.price),0) AS n").
		Joins("LEFT JOIN categories ON categories.id = assets.category_id").
		Where("assets.status <> ?", model.AssetScrapped).
		Group("categories.name").Order("n DESC").Limit(10).Scan(&rows)
	return toNV(rows)
}

func (s *Server) chartByStatus() []nameValue {
	var rows []nameCount
	s.db.Model(&model.Asset{}).Select("status AS name, COUNT(*) AS n").Group("status").Scan(&rows)
	out := make([]nameValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, nameValue{Name: service.StatusLabel(r.Name), Value: r.N})
	}
	return out
}

func (s *Server) chartByDept() []nameValue {
	var rows []nameCount
	s.db.Model(&model.Asset{}).
		Select("departments.name AS name, COUNT(assets.id) AS n").
		Joins("JOIN departments ON departments.id = assets.holder_dept_id").
		Where("assets.status = ?", model.AssetInUse).
		Group("departments.name").Order("n DESC").Scan(&rows)
	return toNV(rows)
}

func (s *Server) chartTopHolders(limit int) []nameValue {
	var rows []nameCount
	s.db.Model(&model.Asset{}).
		Select("users.real_name AS name, COUNT(assets.id) AS n").
		Joins("JOIN users ON users.id = assets.holder_id").
		Where("assets.status = ?", model.AssetInUse).
		Group("users.real_name").Order("n DESC").Limit(limit).Scan(&rows)
	return toNV(rows)
}

// chartMonthlyTrend 近 N 个月的领用 / 归还 / 驳回趋势。
// 按月归并放在 Go 侧做，省得依赖 SQLite 对带时区时间串的 strftime 解析。
func (s *Server) chartMonthlyTrend(months int) gin.H {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -(months - 1), 0)

	labels := make([]string, 0, months)
	index := make(map[string]int, months)
	for i := 0; i < months; i++ {
		key := start.AddDate(0, i, 0).Format("2006-01")
		index[key] = i
		labels = append(labels, key)
	}
	borrow := make([]int, months)
	returned := make([]int, months)
	rejected := make([]int, months)

	var apps []model.Application
	s.db.Select("type", "status", "created_at", "approved_at").
		Where("created_at >= ?", start).Find(&apps)
	for _, a := range apps {
		ts := a.CreatedAt
		if a.ApprovedAt != nil {
			ts = *a.ApprovedAt
		}
		i, ok := index[ts.Format("2006-01")]
		if !ok {
			continue
		}
		switch {
		case a.Status == model.StatusRejected:
			rejected[i]++
		case a.Status == model.StatusApproved && a.Type == model.ApplyBorrow:
			borrow[i]++
		case a.Status == model.StatusApproved && a.Type == model.ApplyReturn:
			returned[i]++
		}
	}
	return gin.H{"labels": labels, "borrow": borrow, "returned": returned, "rejected": rejected}
}

func (s *Server) chartPurchaseByYear() gin.H {
	var assets []model.Asset
	s.db.Select("purchase_date", "price").Where("purchase_date IS NOT NULL").Find(&assets)

	countBy := map[string]int{}
	valueBy := map[string]float64{}
	for _, a := range assets {
		y := a.PurchaseDate.Format("2006")
		countBy[y]++
		valueBy[y] += a.Price
	}
	years := make([]string, 0, len(countBy))
	for y := range countBy {
		years = append(years, y)
	}
	sort.Strings(years)

	counts := make([]int, 0, len(years))
	values := make([]float64, 0, len(years))
	for _, y := range years {
		counts = append(counts, countBy[y])
		values = append(values, round2(valueBy[y]))
	}
	return gin.H{"labels": years, "counts": counts, "values": values}
}

// chartDueForecast 未来 8 周的到期设备数，帮管理员提前排回收计划。
func (s *Server) chartDueForecast() gin.H {
	now := time.Now()
	const weeks = 8

	labels := make([]string, 0, weeks+1)
	counts := make([]int, weeks+1)
	labels = append(labels, "已逾期")
	for i := 0; i < weeks; i++ {
		labels = append(labels, now.AddDate(0, 0, i*7).Format("01/02")+" 起")
	}

	var assets []model.Asset
	s.db.Select("due_at").
		Where("status = ? AND due_at IS NOT NULL AND due_at <= ?",
			model.AssetInUse, now.AddDate(0, 0, weeks*7)).
		Find(&assets)
	for _, a := range assets {
		if a.DueAt.Before(now) {
			counts[0]++
			continue
		}
		if idx := int(a.DueAt.Sub(now).Hours()/24)/7 + 1; idx >= 1 && idx <= weeks {
			counts[idx]++
		}
	}
	return gin.H{"labels": labels, "counts": counts}
}

// statsMine 普通用户首页：只呈现与本人相关的数据。
func (s *Server) statsMine(c *gin.Context) {
	u := middleware.CurrentUser(c)
	now := time.Now()
	days := s.settings.Int("remind_before_days", 7)

	countMine := func(where string, args ...any) int64 {
		var n int64
		s.db.Model(&model.Asset{}).Where("holder_id = ?", u.ID).Where(where, args...).Count(&n)
		return n
	}
	myAssets := countMine("status = ?", model.AssetInUse)

	countApply := func(status string) int64 {
		var n int64
		s.db.Model(&model.Application{}).
			Where("applicant_id = ? AND status = ?", u.ID, status).Count(&n)
		return n
	}

	var myRepair, unread int64
	s.db.Model(&model.RepairTicket{}).
		Where("reporter_id = ? AND status IN ?", u.ID, []string{model.RepairOpen, model.RepairProcessing}).
		Count(&myRepair)
	s.db.Model(&model.Notification{}).Where("user_id = ? AND is_read = ?", u.ID, false).Count(&unread)

	var myValue float64
	s.db.Model(&model.Asset{}).Where("holder_id = ? AND status = ?", u.ID, model.AssetInUse).
		Select("COALESCE(SUM(price),0)").Scan(&myValue)

	var rows []nameCount
	s.db.Model(&model.Asset{}).
		Select("categories.name AS name, COUNT(assets.id) AS n").
		Joins("LEFT JOIN categories ON categories.id = assets.category_id").
		Where("assets.holder_id = ? AND assets.status = ?", u.ID, model.AssetInUse).
		Group("categories.name").Order("n DESC").Scan(&rows)

	// 最近要归还的 5 台设备，直接渲染成待办列表。
	var upcoming []model.Asset
	s.db.Preload("Category").
		Where("holder_id = ? AND status = ? AND due_at IS NOT NULL", u.ID, model.AssetInUse).
		Order("due_at ASC").Limit(5).Find(&upcoming)

	response.OK(c, gin.H{
		"myAssets": myAssets,
		"myValue":  round2(myValue),
		"dueSoon": countMine("status = ? AND due_at IS NOT NULL AND due_at >= ? AND due_at <= ?",
			model.AssetInUse, now, now.AddDate(0, 0, days)),
		"overdue": countMine("status = ? AND due_at IS NOT NULL AND due_at < ?",
			model.AssetInUse, now),
		"pending":    countApply(model.StatusPending),
		"approved":   countApply(model.StatusApproved),
		"rejected":   countApply(model.StatusRejected),
		"myRepair":   myRepair,
		"unread":     unread,
		"byCategory": toNV(rows),
		"upcoming":   toViews(upcoming),
		"remindDays": days,
	})
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
