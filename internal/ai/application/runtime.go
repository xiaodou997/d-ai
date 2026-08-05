package application

import (
	"context"
	"errors"

	"xiaodou/dai/internal/ai/core/identity"
)

var (
	ErrRuntimeInvocationNotFound = errors.New("runtime invocation not found")
	ErrRuntimeAppNotVisible      = errors.New("runtime app not visible")
)

// RuntimePromptBinding is the application-runtime view of a prompt binding
// after its current revision has been resolved.
type RuntimePromptBinding struct {
	PromptID          string
	PromptName        string
	PromptRevision    int
	Role              PromptBindingRole
	BindingOrder      int
	TemplateText      string
	Variables         []string
	VariablesRequired []string
}

// RuntimeApp is the application-runtime view consumed by invoke-key expansion.
type RuntimeApp struct {
	App            App
	PromptBindings []RuntimePromptBinding
}

// RuntimeInvocation is the resolved invoke-key target before the runtime kernel
// performs commercial dispatch and upstream binding.
type RuntimeInvocation struct {
	InvokeKey    InvokeKey
	BoundModelID string
	App          *RuntimeApp
}

// Subject converts a resolved runtime invocation into the normalized runtime
// caller identity used by the rebuilt runtime kernel.
func (i RuntimeInvocation) Subject() identity.Subject {
	return identity.Subject{
		AuthMethod:    identity.AuthMethodInvokeKey,
		RequestSource: identity.RequestSourceInvokeKey,
		Scope:         i.InvokeKey.OwnerScope,
		TenantID:      i.InvokeKey.TenantID,
		UserID:        i.InvokeKey.UserID,
		InvokeKeyID:   i.InvokeKey.ID,
	}
}

// RuntimeResolver expands invoke keys and their optional app targets from the
// application layer.
type RuntimeResolver interface {
	ResolveRuntimeInvocationByKeyHash(ctx context.Context, keyHash string) (RuntimeInvocation, error)
}

// RuntimeIDResolver reloads a persisted invoke-key reference without needing
// the original plaintext key. Async workers use it before every attempt.
type RuntimeIDResolver interface {
	ResolveRuntimeInvocationByID(ctx context.Context, ownerScope identity.Scope, tenantID, userID, keyID string) (RuntimeInvocation, error)
}
