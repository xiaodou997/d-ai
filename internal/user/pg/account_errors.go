package pg

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	authports "xiaodou/dai/internal/auth/ports"
	userports "xiaodou/dai/internal/user/ports"
)

func translateAccountConstraint(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		switch pgErr.ConstraintName {
		case "ux_iam_accounts_username_normalized":
			return authports.ErrUsernameTaken
		case "ux_iam_accounts_email_normalized":
			return authports.ErrEmailTaken
		}
	case "23503":
		if pgErr.ConstraintName == "iam_accounts_tenant_id_fkey" {
			return userports.ErrTenantNotFound
		}
	}
	return err
}
