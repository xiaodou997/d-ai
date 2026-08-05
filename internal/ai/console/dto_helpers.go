package console

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Shared helpers for mapping service-layer domain values back onto the
// compatibility wire DTOs (which embed pgtype values). Reconstructing pgtype
// keeps the JSON shape stable for the remaining callers of this package.

// textPtrToPg renders a *string as pgtype.Text (nil → SQL NULL → JSON null).
func textPtrToPg(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

// pgUUIDFromString rebuilds a pgtype.UUID from the flattened domain string.
// Empty or unparseable input yields an invalid UUID → JSON null, matching how a
// NULL column rendered before the service layer flattened it to "".
func pgUUIDFromString(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}
	}
	return u
}

// int32PtrToPg renders a *int32 as pgtype.Int4 (nil → JSON null).
func int32PtrToPg(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

// timeToMillisPtr renders a time.Time as a *int64 epoch-millis (zero → nil),
// matching the existing millis(pgtype.Timestamptz) behaviour for NULL timestamps.
func timeToMillisPtr(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

// timePtrToMillis renders an optional time as *int64 epoch-millis (nil → nil),
// matching the existing millis(pgtype.Timestamptz) behaviour.
func timePtrToMillis(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

// pgTimestamptzToPtr converts an optional SQL timestamp into a *time.Time for
// passing scope filters into the service layer (invalid → nil = no lower bound).
func pgTimestamptzToPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}
