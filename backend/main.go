package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shiguang-island/backend/internal/config"
	"shiguang-island/backend/internal/httpapi"
	"shiguang-island/backend/internal/store"
	"shiguang-island/backend/migrations"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("服务退出", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := migrations.Run(ctx, st.Pool()); err != nil {
		return err
	}
	logger.Info("数据库迁移完成")

	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		return fmt.Errorf("创建上传目录 %s 失败: %w", cfg.UploadDir, err)
	}

	static, err := newStaticHandler()
	if err != nil {
		return fmt.Errorf("加载内嵌前端失败: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(cfg, st, logger, static),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("拾光星岛已启动", "addr", cfg.Addr, "upload_dir", cfg.UploadDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("收到退出信号，正在关闭服务")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关闭失败: %w", err)
	}
	logger.Info("服务已停止")
	return nil
}
