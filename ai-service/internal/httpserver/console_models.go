package httpserver

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"xiaodou/uni-ai-api/internal/domain"
)

// consoleCapabilityProfile describes how a first-party web console feature
// reaches a model capability: the wire protocol the browser speaks for it and
// whether that protocol is streamed. It is the single source of truth shared
// by the console model picker and (in future) the console runtime endpoints.
type consoleCapabilityProfile struct {
	Capability     domain.CapabilityType
	ClientProtocol domain.UpstreamProtocol
	RequiresStream bool
}

// consoleCapabilityProfiles enumerates the capabilities the web console can
// drive. Only `chat` ships today. Adding a web image feature is one entry:
//
//	domain.CapabilityImage: {domain.CapabilityImage, domain.ProtocolOpenAIImages, false},
//
// after which GET /console/v1/models?capability=image works with no other
// change.
var consoleCapabilityProfiles = map[domain.CapabilityType]consoleCapabilityProfile{
	domain.CapabilityChat: {
		Capability:     domain.CapabilityChat,
		ClientProtocol: domain.ProtocolOpenAIChat,
		RequiresStream: true,
	},
}

type consoleModelDTO struct {
	ModelCode      string `json:"model_code"`
	CapabilityType string `json:"capability_type"`
}

// handleConsoleModels lists the models the JWT caller can actually use in the
// web console for a given capability (default `chat`). A model qualifies when
// it is granted to the caller, of the requested capability, and backed by at
// least one route reachable over that capability's console wire protocol.
//
// This mirrors the hard limit already enforced by the console runtime
// endpoint: a model with no matching route is rejected there with
// no_matching_deployment; here it is simply omitted from the picker.
func (s *Server) handleConsoleModels(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}

	capParam := r.URL.Query().Get("capability")
	if capParam == "" {
		capParam = string(domain.CapabilityChat)
	}
	profile, known := consoleCapabilityProfiles[domain.CapabilityType(capParam)]
	if !known {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "unsupported capability for web console")
		return
	}

	// Granted models of the requested capability, keyed by model UUID.
	codeByID := make(map[pgtype.UUID]string)
	switch identity.OwnerType {
	case domain.OwnerTenant:
		rows, err := s.queries.ListTenantModelGrants(r.Context(), identity.TenantID)
		if err != nil {
			s.logger.Error("console models: list tenant grants failed", zap.Error(err))
			writeDBErr(w, err)
			return
		}
		for _, row := range rows {
			if row.Status == "active" && row.CapabilityType == capParam {
				codeByID[row.ModelID] = row.ModelCode
			}
		}
	case domain.OwnerUser:
		rows, err := s.queries.ListUserAvailableModels(r.Context(), identity.TenantID)
		if err != nil {
			s.logger.Error("console models: list user models failed", zap.Error(err))
			writeDBErr(w, err)
			return
		}
		for _, row := range rows {
			if row.Status != "disabled" && row.GrantStatus == "active" && row.CapabilityType == capParam {
				codeByID[row.ID] = row.ModelCode
			}
		}
	default:
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}

	ids := make([]pgtype.UUID, 0, len(codeByID))
	for id := range codeByID {
		ids = append(ids, id)
	}
	routable, err := s.routeSelector.ModelsWithProtocolRoute(r.Context(), ids, profile.ClientProtocol, profile.RequiresStream)
	if err != nil {
		s.logger.Error("console models: route availability lookup failed", zap.Error(err))
		writeDBErr(w, err)
		return
	}

	out := make([]consoleModelDTO, 0, len(routable))
	for id, code := range codeByID {
		if routable[id] {
			out = append(out, consoleModelDTO{ModelCode: code, CapabilityType: capParam})
		}
	}
	writeOK(w, out)
}
