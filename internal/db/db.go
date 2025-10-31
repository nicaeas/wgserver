package db

import (
	"fmt"
	"time"

	"wgserver/internal/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var xdb *sqlx.DB

func Init(cfg *config.Config) error {
	var err error
	xdb, err = sqlx.Open("mysql", cfg.DBDSN)
	if err != nil {
		return err
	}
	xdb.SetConnMaxLifetime(4 * time.Minute)
	xdb.SetMaxOpenConns(32)
	xdb.SetMaxIdleConns(8)
	return xdb.Ping()
}

func Close() {
	if xdb != nil {
		_ = xdb.Close()
	}
}

func DB() *sqlx.DB { return xdb }

func Tx(fn func(*sqlx.Tx) error) error {
	tx, err := xdb.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// helpers
func Placeholders(n int) string {
	ph := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		ph = append(ph, '?')
		if i != n-1 {
			ph = append(ph, ',')
		}
	}
	return string(ph)
}

func InClause[T any](field string, vals []T) (string, []any) {
	if len(vals) == 0 {
		return "1=0", nil
	}
	q := fmt.Sprintf("%s IN (%s)", field, Placeholders(len(vals)))
	args := make([]any, len(vals))
	for i := range vals {
		args[i] = any(vals[i])
	}
	return q, args
}
