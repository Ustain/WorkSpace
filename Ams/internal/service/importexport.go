package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"ams/internal/model"
)

// ExportColumns 导入与导出共用同一套列定义，保证"导出的文件能直接改完再导回来"。
var ExportColumns = []string{
	"资产编号", "设备名称", "设备类型", "品牌", "型号", "序列号", "规格配置",
	"状态", "存放位置", "采购价格", "采购日期", "保修到期",
	"领用人", "领用部门", "领用时间", "到期时间", "备注",
}

// AssetRow 把一条资产拍平成导出用的字符串行。
func AssetRow(a *model.Asset) []string {
	get := func(p *time.Time) string {
		if p == nil {
			return ""
		}
		return p.Format("2006-01-02")
	}
	cat, holder, dept := "", "", ""
	if a.Category != nil {
		cat = a.Category.Name
	}
	if a.Holder != nil {
		holder = a.Holder.RealName
	}
	if a.HolderDept != nil {
		dept = a.HolderDept.Name
	}
	return []string{
		a.AssetNo, a.Name, cat, a.Brand, a.Model, a.SerialNo, a.Specification,
		StatusLabel(a.Status), a.Location, strconv.FormatFloat(a.Price, 'f', 2, 64),
		get(a.PurchaseDate), get(a.WarrantyUntil),
		holder, dept, get(a.BorrowedAt), get(a.DueAt), a.Remark,
	}
}

// ExportCSV 输出带 BOM 的 UTF-8 CSV，Excel 直接双击打开不会乱码。
func ExportCSV(assets []model.Asset) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	if err := w.Write(ExportColumns); err != nil {
		return nil, err
	}
	for i := range assets {
		if err := w.Write(AssetRow(&assets[i])); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// ExportXLSX 输出带表头样式、冻结首行和自动筛选的 Excel。
func ExportXLSX(assets []model.Asset) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "资产台账"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	head, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"409EFF"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	for i, c := range ExportColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, c)
	}
	lastCol, _ := excelize.ColumnNumberToName(len(ExportColumns))
	f.SetCellStyle(sheet, "A1", lastCol+"1", head)
	f.SetRowHeight(sheet, 1, 24)

	for r := range assets {
		row := AssetRow(&assets[r])
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			// 价格写成数字，方便在 Excel 里直接求和。
			if c == 9 {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					f.SetCellValue(sheet, cell, n)
					continue
				}
			}
			f.SetCellValue(sheet, cell, v)
		}
	}

	widths := []float64{16, 24, 12, 12, 20, 18, 34, 10, 18, 12, 12, 12, 10, 20, 12, 12, 24}
	for i, wd := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, wd)
	}
	f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	f.AutoFilter(sheet, fmt.Sprintf("A1:%s1", lastCol), nil)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportTemplate 生成带示例行和字段说明的导入模板。
func ImportTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "导入模板"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	head, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"67C23A"}},
	})
	for i, c := range ExportColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, c)
	}
	lastCol, _ := excelize.ColumnNumberToName(len(ExportColumns))
	f.SetCellStyle(sheet, "A1", lastCol+"1", head)

	sample := []string{
		"（留空则自动生成）", "ThinkPad X1 Carbon", "笔记本电脑", "Lenovo", "X1C Gen11",
		"SN123456", "i7/32G/1T", "在库", "A座5层办公区", "12800",
		"2025-03-15", "2028-03-15", "", "", "", "", "示例行，导入前请删除",
	}
	for c, v := range sample {
		cell, _ := excelize.CoordinatesToCellName(c+1, 2)
		f.SetCellValue(sheet, cell, v)
	}
	for i := range ExportColumns {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, 18)
	}

	notes := []string{
		"填写说明：",
		"1. 资产编号留空时系统按「类型码-年份-流水号」自动生成；填写且已存在时执行更新（覆盖导入）。",
		"2. 设备类型必须是系统中已存在的类型名称，如：笔记本电脑、台式机、显示器、鼠标、键盘、转接头、投影仪等。",
		"3. 状态可填：在库 / 领用中 / 维修中 / 已报废 / 遗失，留空默认为「在库」。",
		"4. 日期统一使用 2006-01-02 格式。",
		"5. 领用人请填写系统中已存在用户的真实姓名；状态为「领用中」时必须填写领用人与到期时间。",
	}
	nsheet := "填写说明"
	f.NewSheet(nsheet)
	for i, n := range notes {
		f.SetCellValue(nsheet, fmt.Sprintf("A%d", i+1), n)
	}
	f.SetColWidth(nsheet, "A", "A", 110)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------- 导入 ----------

// RowError 单行的校验/写入失败信息，会完整回给前端展示。
type RowError struct {
	Row     int    `json:"row"`
	AssetNo string `json:"assetNo"`
	Reason  string `json:"reason"`
}

// ImportResult 导入结果报告。
type ImportResult struct {
	Total   int        `json:"total"`
	Created int        `json:"created"`
	Updated int        `json:"updated"`
	Failed  int        `json:"failed"`
	Errors  []RowError `json:"errors"`
}

var statusByLabel = map[string]string{
	"在库": model.AssetInStock, "可领用": model.AssetInStock,
	"领用中": model.AssetInUse, "使用中": model.AssetInUse,
	"维修中": model.AssetRepairing, "报废": model.AssetScrapped,
	"已报废": model.AssetScrapped, "遗失": model.AssetLost,
}

// ImportAssets 解析上传文件并逐行写入。
// 单行失败不影响其它行，最终返回完整报告，让用户一次看清所有问题。
func ImportAssets(db *gorm.DB, filename string, r io.Reader, operator *model.User) (*ImportResult, error) {
	rows, err := readRows(filename, r)
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("文件中没有可导入的数据行")
	}

	// 预加载字典，避免逐行查库。
	cats := map[string]model.Category{}
	var catList []model.Category
	db.Find(&catList)
	for _, c := range catList {
		cats[strings.TrimSpace(c.Name)] = c
	}
	users := map[string]model.User{}
	var userList []model.User
	db.Find(&userList)
	for _, u := range userList {
		users[strings.TrimSpace(u.RealName)] = u
	}

	res := &ImportResult{}
	for i, row := range rows[1:] {
		lineNo := i + 2
		if isBlank(row) {
			continue
		}
		res.Total++
		asset, isUpdate, err := buildAsset(db, row, cats, users)
		if err != nil {
			res.Failed++
			res.Errors = append(res.Errors, RowError{Row: lineNo, AssetNo: cell(row, 0), Reason: err.Error()})
			continue
		}
		if isUpdate {
			if err := db.Model(&model.Asset{}).Where("id = ?", asset.ID).Updates(asset).Error; err != nil {
				res.Failed++
				res.Errors = append(res.Errors, RowError{Row: lineNo, AssetNo: asset.AssetNo, Reason: "更新失败：" + err.Error()})
				continue
			}
			res.Updated++
			Record(db, asset.ID, "import", operator, "通过批量导入更新设备信息")
			continue
		}
		if err := db.Create(asset).Error; err != nil {
			res.Failed++
			res.Errors = append(res.Errors, RowError{Row: lineNo, AssetNo: asset.AssetNo, Reason: "写入失败：" + err.Error()})
			continue
		}
		res.Created++
		Record(db, asset.ID, "import", operator, "通过批量导入新增设备")
	}
	return res, nil
}

func buildAsset(db *gorm.DB, row []string, cats map[string]model.Category, users map[string]model.User) (*model.Asset, bool, error) {
	name := strings.TrimSpace(cell(row, 1))
	if name == "" {
		return nil, false, fmt.Errorf("设备名称不能为空")
	}
	catName := strings.TrimSpace(cell(row, 2))
	cat, ok := cats[catName]
	if !ok {
		return nil, false, fmt.Errorf("设备类型「%s」不存在，请先在类型管理中新增", catName)
	}

	status := model.AssetInStock
	if s := strings.TrimSpace(cell(row, 7)); s != "" {
		v, ok := statusByLabel[s]
		if !ok {
			return nil, false, fmt.Errorf("状态「%s」不是合法取值", s)
		}
		status = v
	}

	price := 0.0
	if p := strings.TrimSpace(cell(row, 9)); p != "" {
		v, err := strconv.ParseFloat(strings.TrimPrefix(p, "¥"), 64)
		if err != nil {
			return nil, false, fmt.Errorf("采购价格「%s」不是合法数字", p)
		}
		price = v
	}

	purchase, err := parseDate(cell(row, 10))
	if err != nil {
		return nil, false, fmt.Errorf("采购日期格式错误：%s", cell(row, 10))
	}
	warranty, err := parseDate(cell(row, 11))
	if err != nil {
		return nil, false, fmt.Errorf("保修到期格式错误：%s", cell(row, 11))
	}
	borrowedAt, err := parseDate(cell(row, 14))
	if err != nil {
		return nil, false, fmt.Errorf("领用时间格式错误：%s", cell(row, 14))
	}
	dueAt, err := parseDate(cell(row, 15))
	if err != nil {
		return nil, false, fmt.Errorf("到期时间格式错误：%s", cell(row, 15))
	}

	a := &model.Asset{
		Name: name, CategoryID: cat.ID,
		Brand: strings.TrimSpace(cell(row, 3)), Model: strings.TrimSpace(cell(row, 4)),
		SerialNo: strings.TrimSpace(cell(row, 5)), Specification: strings.TrimSpace(cell(row, 6)),
		Status: status, Location: strings.TrimSpace(cell(row, 8)), Price: price,
		PurchaseDate: purchase, WarrantyUntil: warranty,
		BorrowedAt: borrowedAt, DueAt: dueAt, Remark: strings.TrimSpace(cell(row, 16)),
	}

	if holderName := strings.TrimSpace(cell(row, 12)); holderName != "" {
		u, ok := users[holderName]
		if !ok {
			return nil, false, fmt.Errorf("领用人「%s」在系统中不存在", holderName)
		}
		a.HolderID, a.HolderDeptID = u.ID, u.DepartmentID
	}
	if a.Status == model.AssetInUse {
		if a.HolderID == 0 {
			return nil, false, fmt.Errorf("状态为「领用中」时必须填写领用人")
		}
		if a.DueAt == nil {
			return nil, false, fmt.Errorf("状态为「领用中」时必须填写到期时间")
		}
		if a.BorrowedAt == nil {
			a.BorrowedAt = purchase
		}
	}

	assetNo := strings.TrimSpace(cell(row, 0))
	if assetNo != "" && !strings.HasPrefix(assetNo, "（") {
		var exist model.Asset
		if err := db.Where("asset_no = ?", assetNo).First(&exist).Error; err == nil {
			a.ID, a.AssetNo = exist.ID, assetNo
			return a, true, nil
		}
		a.AssetNo = assetNo
		return a, false, nil
	}
	a.AssetNo = NextAssetNo(db, cat.ID)
	return a, false, nil
}

// readRows 按扩展名选择解析器，csv/xlsx 归一成同一种二维字符串。
func readRows(filename string, r io.Reader) ([][]string, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		cr := csv.NewReader(bytes.NewReader(data))
		cr.FieldsPerRecord = -1
		return cr.ReadAll()
	case ".xlsx", ".xlsm":
		f, err := excelize.OpenReader(r)
		if err != nil {
			return nil, fmt.Errorf("无法解析 Excel 文件：%w", err)
		}
		defer f.Close()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("Excel 中没有工作表")
		}
		return f.GetRows(sheets[0])
	default:
		return nil, fmt.Errorf("仅支持 .xlsx 或 .csv 文件")
	}
}

var dateLayouts = []string{"2006-01-02", "2006/1/2", "2006/01/02", "2006-1-2", "2006.01.02", "20060102", "2006-01-02 15:04:05"}

func parseDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return &t, nil
		}
	}
	// Excel 有时会把日期存成序列号。
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 90000 {
		t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.Local).AddDate(0, 0, int(n))
		return &t, nil
	}
	return nil, fmt.Errorf("invalid date")
}

func cell(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}

func isBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
