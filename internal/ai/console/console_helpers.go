package console

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// decodeAdminJSON strictly decodes a JSON request body, writing a 400 envelope
// and returning false on failure.
func decodeAdminJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		zap.L().Warn("decode runtime request body failed", consoleRequestLogFields(r, zap.Error(err))...)
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}
