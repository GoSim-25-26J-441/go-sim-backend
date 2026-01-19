package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DBOptions struct {
	DSN       string
	ConnectTO time.Duration
	PingTO    time.Duration
}

func OpenDB(ctx context.Context, opt DBOptions) (*pgxpool.Pool, error) {
	if opt.DSN == "" {
		return nil, fmt.Errorf("DB_DSN is not set")
	}
	if opt.ConnectTO == 0 {
		opt.ConnectTO = 5 * time.Second
	}
	if opt.PingTO == 0 {
		opt.PingTO = 2 * time.Second
	}

	cctx, cancel := context.WithTimeout(ctx, opt.ConnectTO)
	defer cancel()

	pool, err := pgxpool.New(cctx, opt.DSN)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	pctx, pcancel := context.WithTimeout(ctx, opt.PingTO)
	defer pcancel()

	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}

	return pool, nil
}
