package ports

// ActivationCredentialResult is the non-persistence result returned after an
// administrator requests a one-time account activation/reset credential.
type ActivationCredentialResult struct {
	Token     string
	ExpiresIn int64
}
