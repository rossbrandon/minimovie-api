package handlers

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	mw "github.com/rossbrandon/minimovie-api/internal/api/middleware"
	"github.com/rossbrandon/minimovie-api/internal/store"
)

func getUserFromContext(ctx context.Context) *store.User {
	return mw.GetUser(ctx)
}

func getSessionHashFromContext(ctx context.Context) string {
	return mw.GetSessionHash(ctx)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}
