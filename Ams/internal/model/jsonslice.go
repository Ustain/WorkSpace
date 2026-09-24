package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONSlice 让字符串数组（如角色权限点）以 JSON 文本存进单列，
// 省去一张多对多关联表，读写都直接拿到 []string。
type JSONSlice []string

func (s JSONSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *JSONSlice) Scan(src any) error {
	if src == nil {
		*s = JSONSlice{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New("model: unsupported type for JSONSlice")
	}
	if len(raw) == 0 {
		*s = JSONSlice{}
		return nil
	}
	return json.Unmarshal(raw, (*[]string)(s))
}

func (s JSONSlice) Has(code string) bool {
	for _, v := range s {
		if v == code || v == "*" {
			return true
		}
	}
	return false
}
