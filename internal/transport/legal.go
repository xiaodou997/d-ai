package transport

import (
	"context"

	"xiaodou/dai/internal/weborigin"
)

func legalDocumentURL(ctx context.Context, document string) string {
	return weborigin.Resolve(ctx, "/legal/"+document)
}
