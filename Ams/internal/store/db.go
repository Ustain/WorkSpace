package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ams/internal/model"
)

// Open 打开（必要时创建）SQLite 数据库并完成建表。
// 使用纯 Go 驱动，无需 CGO / 无需外部数据库，拿到二进制就能跑。
func Open(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	// WAL 模式让读写不互相阻塞，busy_timeout 避免并发写直接报 SQLITE_BUSY。
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		// 关联字段用 0 表示"无"（如设备未被领用时 holder_id = 0），
		// 这与物理外键约束互斥，因此引用完整性放在业务层保证。
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(model.Models()...); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return db, nil
}
