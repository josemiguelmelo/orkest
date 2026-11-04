package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func mapError(err error, notFound, conflict error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23503":
		return notFound
	case "23505", "23001":
		return conflict
	default:
		return err
	}
}
