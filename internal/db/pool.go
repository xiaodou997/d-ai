package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	sharedpgxpool "xiaodou/dai/libs/go/pgxpoolx"
	"xiaodou/dai/internal/config"
)

// Pool 封装 pgxpool.Pool，提供应用级别的数据库访问
type Pool struct {
	*pgxpool.Pool
	logger *zap.Logger
}

// NewPool 创建 pgxpool 连接池
func NewPool(cfg config.DatabaseConfig, logger *zap.Logger) (*Pool, error) {
	pool, err := sharedpgxpool.Open(context.Background(), cfg.URL, sharedpgxpool.Options{
		MaxConns:          int32(cfg.MaxOpenConns),
		MinConns:          int32(cfg.MaxIdleConns),
		MaxConnLifetime:   time.Duration(cfg.ConnMaxLifetime) * time.Minute,
		MaxConnIdleTime:   10 * time.Minute,
		HealthCheckPeriod: time.Minute,
	})
	if err != nil {
		return nil, err
	}

	logger.Info("Database connected via pgxpool",
		zap.String("url", sharedpgxpool.MaskURL(cfg.URL)),
		zap.Int("maxConns", cfg.MaxOpenConns),
	)

	return &Pool{Pool: pool, logger: logger}, nil
}

// Ping 健康检查
func (p *Pool) Ping() error {
	return p.Pool.Ping(context.Background())
}

// Close 关闭连接池
func (p *Pool) Close() {
	p.Pool.Close()
}
