package clientruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"xiaodou/dai/internal/ai/domain"
)

const inspectionBodyLimit = 8 << 20

type inspectionProfile interface {
	clientProfile
	prepareInspection(Inspection) (*WireRequest, error)
	decodeModels([]byte) ([]ModelCard, error)
}

type quotaProfile interface {
	clientProfile
	prepareQuota(Inspection) (*WireRequest, error)
	decodeQuota([]byte, time.Time) ([]domain.CredentialQuota, error)
}

func (r *Runtime) SupportsInspection(provider domain.FixedProviderType, want InspectionWant) bool {
	if r == nil {
		return false
	}
	profile, ok := r.profiles[provider]
	if !ok {
		return false
	}
	switch want {
	case InspectModels:
		_, ok = profile.(inspectionProfile)
	case InspectQuota:
		_, ok = profile.(quotaProfile)
	default:
		return false
	}
	return ok
}

func (r *Runtime) Inspect(ctx context.Context, in Inspection) (InspectionSnapshot, error) {
	if r == nil || r.transport == nil {
		return InspectionSnapshot{}, &Error{
			Code:       ErrorRuntimeNotConfigured,
			SafeDetail: "fixed-provider client runtime is not configured",
		}
	}
	profile, ok := r.profiles[in.Provider]
	if !ok {
		return InspectionSnapshot{}, &Error{
			Code:       ErrorUnsupportedProvider,
			SafeDetail: fmt.Sprintf("fixed-provider client profile %q is not registered", in.Provider),
		}
	}
	if !r.SupportsInspection(in.Provider, in.Want) {
		return InspectionSnapshot{}, &Error{
			Code:            ErrorUnsupportedProvider,
			ProfileRevision: profile.revision(),
			SafeDetail:      "selected client profile does not support the requested inspection",
		}
	}
	if in.Credential.ID == "" || in.Credential.AccessToken == "" {
		return InspectionSnapshot{}, &Error{
			Code:            ErrorInvalidInvocation,
			ProfileRevision: profile.revision(),
			SafeDetail:      "inspection credential is incomplete",
		}
	}

	var prepare func(Inspection) (*WireRequest, error)
	if in.Want == InspectQuota {
		prepare = profile.(quotaProfile).prepareQuota
	} else {
		prepare = profile.(inspectionProfile).prepareInspection
	}
	response, refreshedCredential, err := r.executeInspection(ctx, profile, prepare, in)
	if err != nil {
		return InspectionSnapshot{}, err
	}
	if refreshedCredential.ID != "" {
		in.Credential = refreshedCredential
	}
	if response.Body == nil {
		return InspectionSnapshot{}, &Error{Code: ErrorTransport, ProfileRevision: profile.revision(), SafeDetail: "provider inspection returned no body"}
	}
	defer response.Body.Close()

	snapshot := InspectionSnapshot{
		ProfileRevision: profile.revision(),
		TokenVersion:    in.Credential.TokenVersion,
		ETag:            response.Headers.Get("ETag"),
		Source:          "live",
		ObservedAt:      time.Now().UTC(),
	}
	if response.StatusCode == http.StatusNotModified && in.Want == InspectModels {
		snapshot.NotModified = true
		return snapshot, nil
	}
	if response.StatusCode != http.StatusOK {
		return InspectionSnapshot{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      fmt.Sprintf("provider inspection returned HTTP %d", response.StatusCode),
		}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, inspectionBodyLimit+1))
	if err != nil {
		return InspectionSnapshot{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider model inspection response could not be read",
			Cause:           err,
		}
	}
	if len(body) > inspectionBodyLimit {
		return InspectionSnapshot{}, &Error{
			Code:            ErrorRequestContract,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider model inspection response exceeded the size limit",
		}
	}
	if in.Want == InspectQuota {
		snapshot.Quotas, err = profile.(quotaProfile).decodeQuota(body, snapshot.ObservedAt)
	} else {
		snapshot.Models, err = profile.(inspectionProfile).decodeModels(body)
	}
	if err != nil {
		return InspectionSnapshot{}, &Error{
			Code:            ErrorRequestContract,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider model inspection response violated the client profile",
			Cause:           err,
		}
	}
	return snapshot, nil
}

func (r *Runtime) executeInspection(ctx context.Context, profile clientProfile, prepare func(Inspection) (*WireRequest, error), in Inspection) (*WireResponse, Credential, error) {
	request, err := prepare(in)
	if err != nil {
		return nil, Credential{}, &Error{
			Code:            ErrorRequestContract,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection request violated the client profile",
			Cause:           err,
		}
	}
	response, err := r.transport.Do(ctx, request)
	if err != nil {
		return nil, Credential{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection transport failed",
			Cause:           err,
		}
	}
	if response == nil {
		return nil, Credential{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection returned no response",
		}
	}
	if response.StatusCode != http.StatusUnauthorized || in.Credential.RefreshToken == "" || r.refresher == nil {
		return response, Credential{}, nil
	}
	snapshotAndClose(response.Body, authResponseSnapshotLimit)
	refreshed, err := r.refreshCredential(ctx, in.Credential)
	if err != nil {
		return nil, Credential{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection credential refresh failed",
			Cause:           err,
		}
	}
	in.Credential = refreshed
	request, err = prepare(in)
	if err != nil {
		return nil, Credential{}, &Error{
			Code:            ErrorRequestContract,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection retry violated the client profile",
			Cause:           err,
		}
	}
	response, err = r.transport.Do(ctx, request)
	if err != nil {
		return nil, Credential{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection retry failed",
			Cause:           err,
		}
	}
	if response == nil {
		return nil, Credential{}, &Error{
			Code:            ErrorTransport,
			ProfileRevision: profile.revision(),
			SafeDetail:      "provider inspection retry returned no response",
		}
	}
	return response, refreshed, nil
}

var _ Inspector = (*Runtime)(nil)
