package pg

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsTenantNameTaken(t *testing.T) {
	duplicateName := &pgconn.PgError{Code: "23505", ConstraintName: "ux_iam_tenants_tenant_name"}
	if !IsTenantNameTaken(duplicateName) {
		t.Fatal("expected tenant name unique violation to be recognized")
	}
	if IsTenantNameTaken(errors.New("other error")) {
		t.Fatal("expected unrelated error to be ignored")
	}
}

func TestIsTenantReferenced(t *testing.T) {
	if !IsTenantReferenced(&pgconn.PgError{Code: "23503"}) {
		t.Fatal("expected foreign-key violation to mean tenant is referenced")
	}
	if IsTenantReferenced(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("did not expect unique violation to mean tenant is referenced")
	}
}
