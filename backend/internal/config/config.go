package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 汇总服务运行所需的全部配置，全部来自环境变量。
type Config struct {
	Addr           string
	DatabaseURL    string
	UploadDir      string
	AdminToken     string
	MaxUploadBytes int64
	TrustProxy     bool
}

func Load() (Config, error) {
	// 本地开发时读取 backend/.env；已存在的环境变量优先（容器/生产环境不会被覆盖）。
	_ = loadDotEnv(".env")

	cfg := Config{
		Addr:           env("ADDR", ":8080"),
		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		UploadDir:      env("UPLOAD_DIR", "./uploads"),
		AdminToken:     strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		MaxUploadBytes: 8 << 20,
		TrustProxy:     envBool("TRUST_PROXY", false),
	}

	// 兼容云平台注入的 PORT（优先级低于显式 ADDR）。
	if os.Getenv("ADDR") == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			cfg.Addr = ":" + port
		}
	}

	if v := strings.TrimSpace(os.Getenv("MAX_UPLOAD_MB")); v != "" {
		mb, err := strconv.Atoi(v)
		if err != nil || mb <= 0 || mb > 50 {
			return cfg, fmt.Errorf("MAX_UPLOAD_MB 必须是 1-50 之间的整数")
		}
		cfg.MaxUploadBytes = int64(mb) << 20
	}

	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL 未配置")
	}
	if cfg.AdminToken != "" && len(cfg.AdminToken) < 8 {
		return cfg, errors.New("ADMIN_TOKEN 至少需要 8 位，避免被轻易猜到")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// loadDotEnv 是一个零依赖的 .env 读取器，仅支持 KEY=VALUE 与 # 注释。
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
