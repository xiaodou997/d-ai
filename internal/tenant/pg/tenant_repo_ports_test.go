package pg

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsInvitationCodeTaken(t *testing.T) {
	if !IsInvitationCodeTaken(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("expected unique violation to be retryable")
	}
	if IsInvitationCodeTaken(&pgconn.PgError{Code: "23503"}) {
		t.Fatal("did not expect foreign-key violation to be retryable")
	}
	if IsInvitationCodeTaken(errors.New("database unavailable")) {
		t.Fatal("did not expect an unrelated error to be retryable")
	}
}
