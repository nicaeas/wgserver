package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port   int
	LogDir string
	DBDSN  string
	Env    string
}

func Load() *Config {
	cfg := &Config{
		Port:   8888,
		LogDir: "logs",
		DBDSN:  getenv("MYSQL_DSN", "root:1qaz2wsx@tcp(47.116.127.1:3306)/wgserver?parseTime=true&charset=utf8mb4,utf8"),
		Env:    getenv("APP_ENV", "dev"),
	}
	if v := os.Getenv("PORT"); v != "" {
		var p int
		_, _ = fmt.Sscanf(v, "%d", &p)
		if p > 0 {
			cfg.Port = p
		}
	}
	return cfg
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
