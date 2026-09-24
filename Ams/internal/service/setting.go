package service

import (
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"ams/internal/model"
)

// SettingService 带内存缓存的系统参数读写，避免每次判断"提前几天提醒"都打一次库。
type SettingService struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]string
}

func NewSettingService(db *gorm.DB) *SettingService {
	s := &SettingService{db: db, cache: map[string]string{}}
	s.Reload()
	return s
}

func (s *SettingService) Reload() {
	var rows []model.Setting
	if err := s.db.Find(&rows).Error; err != nil {
		return
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	s.mu.Lock()
	s.cache = m
	s.mu.Unlock()
}

func (s *SettingService) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.cache))
	for k, v := range s.cache {
		out[k] = v
	}
	return out
}

func (s *SettingService) Get(key, def string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.cache[key]; ok && v != "" {
		return v
	}
	return def
}

func (s *SettingService) Int(key string, def int) int {
	if n, err := strconv.Atoi(s.Get(key, "")); err == nil {
		return n
	}
	return def
}

func (s *SettingService) Bool(key string, def bool) bool {
	v := s.Get(key, "")
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

func (s *SettingService) Save(values map[string]string) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for k, v := range values {
			row := model.Setting{Key: k, Value: v, UpdatedAt: time.Now()}
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.Reload()
	return nil
}
