package transport

import (
	"testing"

	"xiaodou/dai/internal/auth"
)

func TestNormalizedOptionalText(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		set, value := normalizedOptionalText(nil)
		if set || value != "" {
			t.Fatalf("expected omitted value, got set=%v value=%q", set, value)
		}
	})

	t.Run("trimmed", func(t *testing.T) {
		input := "  tenant note  "
		set, value := normalizedOptionalText(&input)
		if !set || value != "tenant note" {
			t.Fatalf("expected trimmed value, got set=%v value=%q", set, value)
		}
	})

	t.Run("explicit empty", func(t *testing.T) {
		input := "   "
		set, value := normalizedOptionalText(&input)
		if !set || value != "" {
			t.Fatalf("expected explicit empty value, got set=%v value=%q", set, value)
		}
	})
}

func TestAdminEndUserMutationScopeUsesActorClaims(t *testing.T) {
	if got := adminEndUserTenantScope(auth.NewActor("admin", "", int(auth.UserTypePlatformAdmin))); got != "" {
		t.Fatalf("platform admin scope = %q, want global scope", got)
	}
	if got := adminEndUserTenantScope(auth.NewActor("tenant-user", "tenant-a", int(auth.UserTypeTenant))); got != "tenant-a" {
		t.Fatalf("tenant scope = %q, want tenant-a", got)
	}
}
