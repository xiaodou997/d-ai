package transport

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// 共享 pgtype 转换辅助（原置于已删除的 upstream_deployments.go，账号级路由重构后迁出）。

func parseTransportUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}

func textToStringPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func timestamptzToMillisPtr(t pgtype.Timestamptz) *int64 {
	if !t.Valid || t.Time.IsZero() {
		return nil
	}
	v := t.Time.UnixMilli()
	return &v
}
