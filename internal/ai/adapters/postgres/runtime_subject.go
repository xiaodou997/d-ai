package postgres

import (
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

func runtimeSubjectOwnerType(subject *coreidentity.Subject) domain.OwnerType {
	if subject == nil {
		return domain.OwnerTenant
	}
	switch subject.Scope {
	case coreidentity.ScopeUser:
		return domain.OwnerUser
	default:
		return domain.OwnerTenant
	}
}

func runtimeSubjectRequestSource(subject *coreidentity.Subject) domain.RequestSource {
	if subject == nil {
		return domain.RequestSourceAPIKey
	}
	switch subject.RequestSource {
	case coreidentity.RequestSourceAPIKey:
		return domain.RequestSourceAPIKey
	case coreidentity.RequestSourceWebImage:
		return domain.RequestSourceWebImage
	case coreidentity.RequestSourceWebChat:
		return domain.RequestSourceWebChat
	default:
		return domain.RequestSourceAPIKey
	}
}

func runtimeSubjectAuthMethod(subject *coreidentity.Subject) domain.RuntimeAuthMethod {
	if subject == nil {
		return domain.AuthMethodJWT
	}
	switch subject.AuthMethod {
	case coreidentity.AuthMethodAPIKey:
		return domain.AuthMethodAPIKey
	case coreidentity.AuthMethodJWT:
		return domain.AuthMethodJWT
	default:
		return domain.AuthMethodJWT
	}
}

func runtimeSubjectTenantID(subject *coreidentity.Subject) string {
	if subject == nil {
		return ""
	}
	return subject.TenantID
}

func runtimeSubjectUserID(subject *coreidentity.Subject) string {
	if subject == nil {
		return ""
	}
	return subject.UserID
}
