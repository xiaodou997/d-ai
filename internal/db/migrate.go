package db

import (
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

// Migrate 使用 goose 执行嵌入式迁移文件
// fsys: 嵌入的迁移文件系统（如 migrations.FS）
// dir:  迁移文件在 fsys 中的目录路径（如 "migrations"）
func (p *Pool) Migrate(fsys fs.FS, dir string) error {
	// 从 pgxpool 获取一个标准 *sql.DB（仅用于 goose，迁移完即关闭）
	db := stdlib.OpenDBFromPool(p.Pool)
	defer db.Close()

	goose.SetDialect("postgres")
	goose.SetBaseFS(fsys)

	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}

	p.logger.Info("Database migration completed", zap.String("dir", dir))
	return nil
}

// MigrateStatus 查看当前迁移状态（调试用）
func (p *Pool) MigrateStatus(fsys fs.FS, dir string) error {
	db := stdlib.OpenDBFromPool(p.Pool)
	defer db.Close()

	goose.SetDialect("postgres")
	goose.SetBaseFS(fsys)
	return goose.Status(db, dir)
}

// MigrateDown 回滚最后一次迁移（谨慎使用）
func (p *Pool) MigrateDown(fsys fs.FS, dir string) error {
	db := stdlib.OpenDBFromPool(p.Pool)
	defer db.Close()

	goose.SetDialect("postgres")
	goose.SetBaseFS(fsys)
	return goose.Down(db, dir)
}
