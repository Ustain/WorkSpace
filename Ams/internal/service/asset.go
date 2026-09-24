package service

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"ams/internal/model"
)

// Record 追加一条资产履历。履历是只增不改的，构成设备的完整生命周期时间轴。
func Record(db *gorm.DB, assetID uint, action string, op *model.User, detail string) {
	r := model.AssetRecord{
		AssetID: assetID, Action: action, Detail: detail, CreatedAt: time.Now(),
	}
	if op != nil {
		r.Operator, r.OperatorID = op.RealName, op.ID
		if r.Operator == "" {
			r.Operator = op.Username
		}
	} else {
		r.Operator = "系统"
	}
	db.Create(&r)
}

// NextAssetNo 生成形如 NB-2026-0042 的资产编号：类型码 + 年份 + 该类型年度流水。
func NextAssetNo(db *gorm.DB, categoryID uint) string {
	var cat model.Category
	code := "AS"
	if err := db.First(&cat, categoryID).Error; err == nil && cat.Code != "" {
		code = cat.Code
	}
	year := time.Now().Format("2006")
	prefix := fmt.Sprintf("%s-%s-", code, year)

	var last model.Asset
	seq := 1
	if err := db.Where("asset_no LIKE ?", prefix+"%").
		Order("asset_no DESC").First(&last).Error; err == nil {
		var n int
		if _, err := fmt.Sscanf(last.AssetNo[len(prefix):], "%04d", &n); err == nil {
			seq = n + 1
		}
	}
	return fmt.Sprintf("%s%04d", prefix, seq)
}

// AssetStatusLabels 状态中文名，供后端生成履历文案与导出文件复用。
var AssetStatusLabels = map[string]string{
	model.AssetInStock:   "在库",
	model.AssetInUse:     "领用中",
	model.AssetRepairing: "维修中",
	model.AssetScrapped:  "已报废",
	model.AssetLost:      "遗失",
}

var ApplyTypeLabels = map[string]string{
	model.ApplyBorrow: "领用",
	model.ApplyReturn: "归还",
	model.ApplyRenew:  "续借",
	model.ApplyScrap:  "报废",
}

var ApplyStatusLabels = map[string]string{
	model.StatusPending:   "待审批",
	model.StatusApproved:  "已通过",
	model.StatusRejected:  "已驳回",
	model.StatusCancelled: "已撤销",
}

func StatusLabel(s string) string {
	if v, ok := AssetStatusLabels[s]; ok {
		return v
	}
	return s
}
