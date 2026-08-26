package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/dai/internal/ai/db/gen"
)

// numericToFloat converts a pgtype.Numeric to float64 (0 for NULL/invalid).
// Used by the pricing layer where NUMERIC columns hold USD prices/multipliers.
func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// numericToFloatPtr converts a pgtype.Numeric to *float64 (nil for NULL/invalid).
// Used where a NULL multiplier means "inherit / unset".
func numericToFloatPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// floatPtrToNumeric converts a *float64 to pgtype.Numeric (NULL when nil).
func floatPtrToNumeric(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	return floatToNumeric(*v)
}

// nullableUUID parses an optional UUID string into pgtype.UUID; "" → NULL.
func nullableUUID(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	u, err := akUUID(s)
	if err != nil {
		return pgtype.UUID{}
	}
	return u
}

// floatToNumeric converts a float64 to pgtype.Numeric via its decimal string
// form ('f' format never uses exponent, which pgtype.Numeric.Scan rejects).
// Postgres coerces the value to each column's declared scale on write.
func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))
	return n
}

// hashAPIKey returns the hex-encoded SHA-256 of the raw API key token.
// This must match the hash stored in ai_api_keys.key_hash.
func hashAPIKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// uuidToString converts a pgtype.UUID to its standard string representation.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// mustParseUUID parses a UUID string into pgtype.UUID, panicking on invalid input.
// Only used for known-valid UUIDs from the domain layer.
func mustParseUUID(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}
	}
	return u
}

// nullableText converts a string to pgtype.Text (null if empty).
func nullableText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// nullableInt4 converts an int to pgtype.Int4 (null if zero).
func nullableInt4(n int) pgtype.Int4 {
	if n == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

// nullableInt4WithValid converts an int to pgtype.Int4 using an explicit
// validity flag, so callers can preserve zero-duration values when the timing
// point was actually measured.
func nullableInt4WithValid(n int, valid bool) pgtype.Int4 {
	if !valid {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

// unmarshalStringMap decodes a JSONB []byte into map[string]string.
func unmarshalStringMap(b []byte) map[string]string {
	if len(b) == 0 {
		return nil
	}
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	return m
}

// int32PtrToInt4 converts a *int32 to pgtype.Int4 (null when nil).
func int32PtrToInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func existsByID(ctx context.Context, pool dbgen.DBTX, table string, id any) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1)", table)
	var exists bool
	if err := pool.QueryRow(ctx, query, id).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func countOne(ctx context.Context, pool dbgen.DBTX, query string, args ...any) (int, error) {
	var n int
	if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
