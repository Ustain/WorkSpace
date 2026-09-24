// Command ams 是一套面向科研机构 / 事业单位的 IT 办公设备资产管理系统。
//
// 整个系统编译为单个可执行文件：内嵌前端页面、内嵌 SQLite（纯 Go 驱动，无需 CGO），
// 不依赖任何外部数据库或 Node 构建环境，`go run .` 即可完整演示。
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ams/internal/api"
	"ams/internal/auth"
	"ams/internal/config"
	"ams/internal/service"
	"ams/internal/store"
)

//go:embed all:web
var webAssets embed.FS

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	cfg := config.Load()

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err := store.Seed(db); err != nil {
		log.Fatalf("演示数据初始化失败: %v", err)
	}

	settings := service.NewSettingService(db)
	notifier := service.NewNotifier(db, settings, cfg.SMTP)
	workflow := service.NewWorkflow(db, notifier, settings)
	reminder := service.NewReminder(db, notifier, settings)
	jwtMgr := auth.NewManager(cfg.JWTSecret, cfg.TokenTTL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	reminder.Start(ctx, cfg.ReminderInterval)

	webFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	handler := api.NewServer(db, cfg, jwtMgr, settings, notifier, workflow, reminder).Router(webFS)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		printBanner(cfg)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到退出信号，正在关闭服务…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭超时: %v", err)
	}
	log.Println("服务已退出")
}

// printBanner 打印启动信息。刻意不画对齐的框线：中英文混排下终端字宽不一致，
// 框线反而更容易错位。
func printBanner(cfg *config.Config) {
	mail := "模拟投递（未配置 SMTP，邮件内容可在系统内「邮件记录」查看）"
	if cfg.SMTP.Enabled() {
		mail = "真实投递 via " + cfg.SMTP.Host
	}
	fmt.Printf(`
==========================================================
  IT 资产管理系统 AMS 已启动
----------------------------------------------------------
  访问地址：http://localhost%s
  数据文件：%s
  邮件通道：%s
  到期扫描：每 %s 自动执行一次
----------------------------------------------------------
  演示账号（密码 / 角色）
    admin    / admin123   系统管理员
    assetmgr / admin123   资产管理员
    zhouheng / user123    普通用户
==========================================================

`, cfg.Addr, cfg.DBPath, mail, cfg.ReminderInterval.String())
}
