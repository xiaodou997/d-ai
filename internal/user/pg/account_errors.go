package pg

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	authports "xiaodou/dai/internal/auth/ports"
)

func translateAccountConstraint(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "ux_iam_accounts_username_normalized":
			return authports.ErrUsernameTaken
		case "ux_iam_accounts_email_normalized":
			return authports.ErrEmailTaken
		}
	}
	return err
}
