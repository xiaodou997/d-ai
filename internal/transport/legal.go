package transport

import (
	"strings"

	"xiaodou/dai/internal/config"
)

func legalDocumentURL(legal config.LegalConfig, document string) string {
	return strings.TrimRight(legal.BaseURL, "/") + "/" + document
}
