package httpserver

import (
	"go.uber.org/zap"
	"net/http"
)

// handleAuthCallback exchanges an OAuth2 authorization code for a token pair.
// Called by business frontends after URM redirects back with ?code=...
// GET /api/auth/callback?code=<auth_code>&redirect_uri=<callback_url>
func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	redirectURI := r.URL.Query().Get("redirect_uri")
	if code == "" || redirectURI == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "missing code or redirect_uri")
		return
	}

	tokenPair, err := s.urmClient.ExchangeCode(r.Context(), code, redirectURI)
	if err != nil {
		s.logger.Warn("auth callback code exchange failed", zap.Error(err))
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid or expired code")
		return
	}

	writeOK(w, tokenPair)
}
