package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"
	"gorm.io/gorm"

	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
	"ams/internal/service"
)

// 白名单排序字段，避免把用户输入直接拼进 ORDER BY。
var assetSortable = map[string]string{
	"createdAt": "created_at", "assetNo": "asset_no", "name": "name",
	"price": "price", "dueAt": "due_at", "borrowedAt": "borrowed_at",
	"purchaseDate": "purchase_date", "status": "status",
}

// buildAssetQuery 把查询串翻译成 GORM 条件，需求 4 的所有过滤维度都在这里。
func (s *Server) buildAssetQuery(c *gin.Context) *gorm.DB {
	q := s.db.Model(&model.Asset{})

	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("asset_no LIKE ? OR name LIKE ? OR serial_no LIKE ? OR brand LIKE ? OR model LIKE ? OR specification LIKE ?",
			like, like, like, like, like, like)
	}
	// 类型 / 状态支持多选，前端传 status=in_use&status=repairing 或逗号分隔。
	if cats := multiQuery(c, "categoryId"); len(cats) > 0 {
		q = q.Where("category_id IN ?", cats)
	}
	if sts := multiQuery(c, "status"); len(sts) > 0 {
		q = q.Where("status IN ?", sts)
	}
	if v := c.Query("location"); v != "" {
		q = q.Where("location LIKE ?", "%"+v+"%")
	}
	if v := c.Query("holderId"); v != "" {
		q = q.Where("holder_id = ?", v)
	}
	if v := c.Query("deptId"); v != "" {
		q = q.Where("holder_dept_id = ?", v)
	}
	if v := c.Query("brand"); v != "" {
		q = q.Where("brand = ?", v)
	}

	// 领用时间范围
	if t := parseDay(c.Query("borrowedFrom"), false); t != nil {
		q = q.Where("borrowed_at >= ?", t)
	}
	if t := parseDay(c.Query("borrowedTo"), true); t != nil {
		q = q.Where("borrowed_at <= ?", t)
	}
	// 到期（过期）时间范围
	if t := parseDay(c.Query("dueFrom"), false); t != nil {
		q = q.Where("due_at >= ?", t)
	}
	if t := parseDay(c.Query("dueTo"), true); t != nil {
		q = q.Where("due_at <= ?", t)
	}
	// 采购日期范围
	if t := parseDay(c.Query("purchaseFrom"), false); t != nil {
		q = q.Where("purchase_date >= ?", t)
	}
	if t := parseDay(c.Query("purchaseTo"), true); t != nil {
		q = q.Where("purchase_date <= ?", t)
	}
	if v := c.Query("priceMin"); v != "" {
		q = q.Where("price >= ?", v)
	}
	if v := c.Query("priceMax"); v != "" {
		q = q.Where("price <= ?", v)
	}

	now := time.Now()
	switch c.Query("quick") {
	case "overdue": // 已逾期未归还
		q = q.Where("status = ? AND due_at IS NOT NULL AND due_at < ?", model.AssetInUse, now)
	case "dueSoon": // 未来 N 天内到期
		days := s.settings.Int("remind_before_days", 7)
		if v, err := strconv.Atoi(c.Query("days")); err == nil && v > 0 {
			days = v
		}
		q = q.Where("status = ? AND due_at IS NOT NULL AND due_at >= ? AND due_at <= ?",
			model.AssetInUse, now, now.AddDate(0, 0, days))
	case "available":
		q = q.Where("status = ?", model.AssetInStock)
	case "warrantyExpiring": // 保修将在 90 天内到期
		q = q.Where("warranty_until IS NOT NULL AND warranty_until >= ? AND warranty_until <= ?",
			now, now.AddDate(0, 0, 90))
	}

	// 普通用户的"我的设备"视图。
	if c.Query("mine") == "1" {
		if u := middleware.CurrentUser(c); u != nil {
			q = q.Where("holder_id = ?", u.ID)
		}
	}
	return q
}

func multiQuery(c *gin.Context, key string) []string {
	var out []string
	for _, v := range c.QueryArray(key) {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// assetView 在实体基础上补充前端要用的派生字段（是否逾期、剩余天数等）。
type assetView struct {
	model.Asset
	StatusLabel string `json:"statusLabel"`
	IsOverdue   bool   `json:"isOverdue"`
	DaysLeft    *int   `json:"daysLeft"`
}

func toViews(assets []model.Asset) []assetView {
	now := time.Now()
	out := make([]assetView, 0, len(assets))
	for i := range assets {
		a := assets[i]
		v := assetView{Asset: a, StatusLabel: service.StatusLabel(a.Status)}
		if a.Status == model.AssetInUse && a.DueAt != nil {
			d := int(a.DueAt.Sub(now).Hours() / 24)
			v.DaysLeft = &d
			v.IsOverdue = a.DueAt.Before(now)
		}
		out = append(out, v)
	}
	return out
}

func (s *Server) listAssets(c *gin.Context) {
	page, size, offset := pagination(c)
	q := s.buildAssetQuery(c)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		response.Error(c, "查询失败："+err.Error())
		return
	}
	order := assetSortable[c.DefaultQuery("sortBy", "createdAt")]
	if order == "" {
		order = "created_at"
	}
	if strings.ToLower(c.DefaultQuery("sortOrder", "desc")) == "asc" {
		order += " ASC"
	} else {
		order += " DESC"
	}

	var assets []model.Asset
	if err := q.Preload("Category").Preload("Holder").Preload("HolderDept").
		Order(order).Limit(size).Offset(offset).Find(&assets).Error; err != nil {
		response.Error(c, "查询失败："+err.Error())
		return
	}
	response.OKPage(c, toViews(assets), total, page, size)
}

func (s *Server) getAsset(c *gin.Context) {
	var a model.Asset
	if err := s.db.Preload("Category").Preload("Holder").Preload("HolderDept").
		First(&a, idParam(c)).Error; err != nil {
		response.NotFound(c, "设备不存在")
		return
	}
	views := toViews([]model.Asset{a})
	response.OK(c, views[0])
}

type assetReq struct {
	AssetNo       string  `json:"assetNo"`
	Name          string  `json:"name" binding:"required"`
	CategoryID    uint    `json:"categoryId" binding:"required"`
	Brand         string  `json:"brand"`
	Model         string  `json:"model"`
	SerialNo      string  `json:"serialNo"`
	Specification string  `json:"specification"`
	Status        string  `json:"status"`
	Location      string  `json:"location"`
	Price         float64 `json:"price"`
	PurchaseDate  string  `json:"purchaseDate"`
	WarrantyUntil string  `json:"warrantyUntil"`
	Remark        string  `json:"remark"`
}

func (s *Server) createAsset(c *gin.Context) {
	var req assetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "设备名称与类型为必填项")
		return
	}
	op := middleware.CurrentUser(c)
	a := model.Asset{
		AssetNo: strings.TrimSpace(req.AssetNo), Name: req.Name, CategoryID: req.CategoryID,
		Brand: req.Brand, Model: req.Model, SerialNo: req.SerialNo, Specification: req.Specification,
		Status: orDefault(req.Status, model.AssetInStock), Location: req.Location, Price: req.Price,
		PurchaseDate: parseDay(req.PurchaseDate, false), WarrantyUntil: parseDay(req.WarrantyUntil, false),
		Remark: req.Remark,
	}
	if a.AssetNo == "" {
		a.AssetNo = service.NextAssetNo(s.db, a.CategoryID)
	}
	if err := s.db.Create(&a).Error; err != nil {
		response.BadRequest(c, "新增失败，资产编号可能已存在："+err.Error())
		return
	}
	service.Record(s.db, a.ID, "created", op,
		fmt.Sprintf("新增设备「%s」，初始状态 %s", a.Name, service.StatusLabel(a.Status)))
	response.OK(c, a)
}

func (s *Server) updateAsset(c *gin.Context) {
	id := idParam(c)
	var old model.Asset
	if err := s.db.First(&old, id).Error; err != nil {
		response.NotFound(c, "设备不存在")
		return
	}
	var req assetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "设备名称与类型为必填项")
		return
	}
	op := middleware.CurrentUser(c)
	updates := map[string]any{
		"name": req.Name, "category_id": req.CategoryID, "brand": req.Brand,
		"model": req.Model, "serial_no": req.SerialNo, "specification": req.Specification,
		"location": req.Location, "price": req.Price, "remark": req.Remark,
		"purchase_date": parseDay(req.PurchaseDate, false), "warranty_until": parseDay(req.WarrantyUntil, false),
	}
	if req.AssetNo != "" {
		updates["asset_no"] = req.AssetNo
	}
	// 设备正被领用时，不允许直接改状态绕过归还流程。
	if req.Status != "" && req.Status != old.Status {
		if old.Status == model.AssetInUse && req.Status != model.AssetLost {
			response.BadRequest(c, "设备正在领用中，请通过归还流程变更状态")
			return
		}
		updates["status"] = req.Status
	}
	if err := s.db.Model(&model.Asset{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Error(c, "保存失败："+err.Error())
		return
	}
	service.Record(s.db, id, "update", op, describeChanges(&old, req))
	response.OK(c, gin.H{"message": "设备信息已更新"})
}

// describeChanges 生成人话版的变更说明，写进资产履历。
func describeChanges(old *model.Asset, req assetReq) string {
	var parts []string
	add := func(label, a, b string) {
		if a != b {
			parts = append(parts, fmt.Sprintf("%s：%s → %s", label, orDash(a), orDash(b)))
		}
	}
	add("名称", old.Name, req.Name)
	add("品牌", old.Brand, req.Brand)
	add("型号", old.Model, req.Model)
	add("序列号", old.SerialNo, req.SerialNo)
	add("存放位置", old.Location, req.Location)
	add("规格", old.Specification, req.Specification)
	if req.Status != "" {
		add("状态", service.StatusLabel(old.Status), service.StatusLabel(req.Status))
	}
	if old.Price != req.Price {
		parts = append(parts, fmt.Sprintf("采购价：¥%.2f → ¥%.2f", old.Price, req.Price))
	}
	if len(parts) == 0 {
		return "编辑设备信息（无实质变更）"
	}
	return "编辑设备信息：" + strings.Join(parts, "；")
}

func (s *Server) deleteAsset(c *gin.Context) {
	id := idParam(c)
	var a model.Asset
	if err := s.db.First(&a, id).Error; err != nil {
		response.NotFound(c, "设备不存在")
		return
	}
	if a.Status == model.AssetInUse {
		response.BadRequest(c, "设备正在领用中，无法删除。请先完成归还流程")
		return
	}
	if err := s.db.Delete(&model.Asset{}, id).Error; err != nil {
		response.Error(c, "删除失败")
		return
	}
	s.db.Where("asset_id = ?", id).Delete(&model.AssetRecord{})
	response.OK(c, gin.H{"message": "设备已删除"})
}

type batchReq struct {
	IDs      []uint `json:"ids" binding:"required"`
	Action   string `json:"action" binding:"required"` // scrap / repair / instock / transfer / delete
	Location string `json:"location"`
	Remark   string `json:"remark"`
}

// batchAssets 批量报废/送修/入库/调拨/删除，处理大批量台账时省去逐条点击。
func (s *Server) batchAssets(c *gin.Context) {
	var req batchReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		response.BadRequest(c, "请选择要操作的设备")
		return
	}
	op := middleware.CurrentUser(c)

	var assets []model.Asset
	s.db.Where("id IN ?", req.IDs).Find(&assets)
	if len(assets) == 0 {
		response.BadRequest(c, "未找到对应设备")
		return
	}

	var affected int
	var skipped []string
	for _, a := range assets {
		// 领用中的设备不能被批量改状态或删除，必须走归还流程。
		if a.Status == model.AssetInUse && req.Action != "transfer" {
			skipped = append(skipped, a.AssetNo)
			continue
		}
		var err error
		var detail string
		switch req.Action {
		case "scrap":
			err = s.db.Model(&model.Asset{}).Where("id = ?", a.ID).
				Updates(map[string]any{"status": model.AssetScrapped, "remark": orDefault(req.Remark, a.Remark)}).Error
			detail = "批量报废。" + req.Remark
		case "repair":
			err = s.db.Model(&model.Asset{}).Where("id = ?", a.ID).Update("status", model.AssetRepairing).Error
			detail = "批量转入维修中"
		case "instock":
			err = s.db.Model(&model.Asset{}).Where("id = ?", a.ID).Update("status", model.AssetInStock).Error
			detail = "批量恢复为在库"
		case "transfer":
			if req.Location == "" {
				response.BadRequest(c, "调拨操作需要指定目标位置")
				return
			}
			err = s.db.Model(&model.Asset{}).Where("id = ?", a.ID).Update("location", req.Location).Error
			detail = fmt.Sprintf("批量调拨：%s → %s", orDash(a.Location), req.Location)
		case "delete":
			err = s.db.Delete(&model.Asset{}, a.ID).Error
			detail = "批量删除"
		default:
			response.BadRequest(c, "不支持的批量操作")
			return
		}
		if err != nil {
			continue
		}
		affected++
		if req.Action != "delete" {
			service.Record(s.db, a.ID, req.Action, op, detail)
		}
	}
	msg := fmt.Sprintf("已处理 %d 台设备", affected)
	if len(skipped) > 0 {
		msg += fmt.Sprintf("；%d 台因处于领用中被跳过（%s）", len(skipped), strings.Join(skipped, "、"))
	}
	response.OK(c, gin.H{"message": msg, "affected": affected, "skipped": skipped})
}

func (s *Server) assetRecords(c *gin.Context) {
	var records []model.AssetRecord
	if err := s.db.Where("asset_id = ?", idParam(c)).
		Order("created_at DESC, id DESC").Limit(200).Find(&records).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}
	response.OK(c, records)
}

// assetQRCode 生成资产标签二维码，扫码即可跳到该设备详情页，方便现场盘点。
func (s *Server) assetQRCode(c *gin.Context) {
	var a model.Asset
	if err := s.db.First(&a, idParam(c)).Error; err != nil {
		response.NotFound(c, "设备不存在")
		return
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	target := fmt.Sprintf("%s://%s/#/assets?keyword=%s", scheme, c.Request.Host, url.QueryEscape(a.AssetNo))
	png, err := qrcode.Encode(target, qrcode.Medium, 320)
	if err != nil {
		response.Error(c, "生成二维码失败")
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

// assetOptions 下拉框数据源：类型、状态、位置、品牌、可选领用人。
func (s *Server) assetOptions(c *gin.Context) {
	var cats []model.Category
	s.db.Order("sort ASC, id ASC").Find(&cats)

	var locations, brands []string
	s.db.Model(&model.Asset{}).Distinct().Where("location <> ''").Order("location").Pluck("location", &locations)
	s.db.Model(&model.Asset{}).Distinct().Where("brand <> ''").Order("brand").Pluck("brand", &brands)

	statuses := make([]gin.H, 0, len(service.AssetStatusLabels))
	for _, code := range []string{model.AssetInStock, model.AssetInUse, model.AssetRepairing, model.AssetScrapped, model.AssetLost} {
		statuses = append(statuses, gin.H{"value": code, "label": service.AssetStatusLabels[code]})
	}

	var depts []model.Department
	s.db.Order("id").Find(&depts)

	response.OK(c, gin.H{
		"categories": cats, "statuses": statuses,
		"locations": locations, "brands": brands, "departments": depts,
	})
}

// ---------- 导入 / 导出 ----------

func (s *Server) exportAssets(c *gin.Context) {
	var assets []model.Asset
	if err := s.buildAssetQuery(c).Preload("Category").Preload("Holder").Preload("HolderDept").
		Order("asset_no ASC").Find(&assets).Error; err != nil {
		response.Error(c, "导出失败："+err.Error())
		return
	}

	stamp := time.Now().Format("20060102-150405")
	if c.DefaultQuery("format", "xlsx") == "csv" {
		data, err := service.ExportCSV(assets)
		if err != nil {
			response.Error(c, "导出失败")
			return
		}
		sendFile(c, fmt.Sprintf("资产台账-%s.csv", stamp), "text/csv; charset=utf-8", data)
		return
	}
	data, err := service.ExportXLSX(assets)
	if err != nil {
		response.Error(c, "导出失败："+err.Error())
		return
	}
	sendFile(c, fmt.Sprintf("资产台账-%s.xlsx", stamp),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (s *Server) importTemplate(c *gin.Context) {
	data, err := service.ImportTemplate()
	if err != nil {
		response.Error(c, "生成模板失败")
		return
	}
	sendFile(c, "资产导入模板.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (s *Server) importAssets(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请选择要导入的文件")
		return
	}
	if fh.Size > 10<<20 {
		response.BadRequest(c, "文件不能超过 10MB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		response.BadRequest(c, "文件读取失败")
		return
	}
	defer f.Close()

	res, err := service.ImportAssets(s.db, fh.Filename, f, middleware.CurrentUser(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, res)
}

// sendFile 统一处理下载响应头，filename* 让中文文件名在各浏览器下都正常。
func sendFile(c *gin.Context, filename, contentType string, data []byte) {
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
			url.PathEscape(filename), url.PathEscape(filename)))
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.Data(http.StatusOK, contentType, data)
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func orDash(s string) string {
	if s == "" {
		return "空"
	}
	return s
}
