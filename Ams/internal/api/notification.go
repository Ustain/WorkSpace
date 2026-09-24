package api

import (
	"github.com/gin-gonic/gin"

	"ams/internal/middleware"
	"ams/internal/model"
	"ams/internal/response"
)

func (s *Server) listNotifications(c *gin.Context) {
	page, size, offset := pagination(c)
	u := middleware.CurrentUser(c)

	q := s.db.Model(&model.Notification{}).Where("user_id = ?", u.ID)
	if c.Query("unread") == "1" {
		q = q.Where("is_read = ?", false)
	}
	if v := multiQuery(c, "type"); len(v) > 0 {
		q = q.Where("type IN ?", v)
	}

	var total int64
	q.Count(&total)

	var list []model.Notification
	if err := q.Order("is_read ASC, created_at DESC").Limit(size).Offset(offset).Find(&list).Error; err != nil {
		response.Error(c, "查询失败")
		return
	}
	response.OKPage(c, list, total, page, size)
}

func (s *Server) unreadCount(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var total int64
	s.db.Model(&model.Notification{}).Where("user_id = ? AND is_read = ?", u.ID, false).Count(&total)

	// 按类型拆开，前端可以给"到期提醒"单独标红。
	type row struct {
		Type string
		N    int64
	}
	var rows []row
	s.db.Model(&model.Notification{}).
		Select("type, COUNT(*) AS n").
		Where("user_id = ? AND is_read = ?", u.ID, false).
		Group("type").Scan(&rows)

	byType := map[string]int64{}
	for _, r := range rows {
		byType[r.Type] = r.N
	}
	response.OK(c, gin.H{"total": total, "byType": byType})
}

func (s *Server) readNotification(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if err := s.db.Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", idParam(c), u.ID).
		Update("is_read", true).Error; err != nil {
		response.Error(c, "操作失败")
		return
	}
	response.OK(c, gin.H{"message": "已标记为已读"})
}

func (s *Server) readAllNotifications(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if err := s.db.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", u.ID, false).
		Update("is_read", true).Error; err != nil {
		response.Error(c, "操作失败")
		return
	}
	response.OK(c, gin.H{"message": "全部消息已标记为已读"})
}

func (s *Server) deleteNotification(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if err := s.db.Where("id = ? AND user_id = ?", idParam(c), u.ID).
		Delete(&model.Notification{}).Error; err != nil {
		response.Error(c, "删除失败")
		return
	}
	response.OK(c, gin.H{"message": "消息已删除"})
}
