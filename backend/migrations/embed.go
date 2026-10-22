package migrations

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var files embed.FS

// advisoryLockID 防止多实例同时启动时重复执行迁移。
const advisoryLockID int64 = 0x5A17_15A1

// Run 按文件名顺序执行尚未应用的迁移，并记录在 schema_migrations 中。
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("获取迁移锁失败: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("创建迁移记录表失败: %w", err)
	}

	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("检查迁移 %s 失败: %w", name, err)
		}
		if applied {
			continue
		}

		body, err := files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("读取迁移 %s 失败: %w", name, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		// SQL 文件包含多条语句，必须使用 simple protocol。
		if _, err := tx.Exec(ctx, string(body), pgx.QueryExecModeSimpleProtocol); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("执行迁移 %s 失败: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("记录迁移 %s 失败: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("提交迁移 %s 失败: %w", name, err)
		}
	}
	return nil
}
