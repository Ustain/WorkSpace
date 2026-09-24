package config

import (
	"os"
	"strconv"
	"time"
)

// Config 汇总所有运行期可调参数，全部支持环境变量覆盖，便于演示与部署。
type Config struct {
	Addr      string
	DBPath    string
	JWTSecret string
	TokenTTL  time.Duration

	// 到期提醒扫描周期。演示时可调小以便快速看到效果。
	ReminderInterval time.Duration

	SMTP SMTPConfig
}

// SMTPConfig 未配置 Host 时，邮件进入"模拟投递"模式：不真正外发，
// 但完整落库到 email_logs，前端可查看投递内容，保证演示链路完整。
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func (s SMTPConfig) Enabled() bool { return s.Host != "" }

func Load() *Config {
	return &Config{
		Addr:             env("AMS_ADDR", ":8080"),
		DBPath:           env("AMS_DB", "data/ams.db"),
		JWTSecret:        env("AMS_JWT_SECRET", "ams-demo-secret-change-me"),
		TokenTTL:         time.Duration(envInt("AMS_TOKEN_TTL_HOURS", 12)) * time.Hour,
		ReminderInterval: time.Duration(envInt("AMS_REMINDER_INTERVAL_MINUTES", 30)) * time.Minute,
		SMTP: SMTPConfig{
			Host:     env("AMS_SMTP_HOST", ""),
			Port:     envInt("AMS_SMTP_PORT", 465),
			Username: env("AMS_SMTP_USER", ""),
			Password: env("AMS_SMTP_PASS", ""),
			From:     env("AMS_SMTP_FROM", "ams-noreply@example.com"),
		},
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
