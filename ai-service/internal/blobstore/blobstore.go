// Package blobstore provides content-addressed storage for large binary objects
// extracted from AI request/response payloads (images, audio, etc.).
// Blobs are keyed by their hex SHA-256 digest; identical content is stored once.
package blobstore

import "context"

// BlobStore persists binary blobs with content-type metadata.
// Implementations must be safe for concurrent use.
type BlobStore interface {
	// Put stores data under its sha256 hex digest. If the digest already
	// exists the call is a no-op (ON CONFLICT DO NOTHING semantics).
	// contentType is e.g. "image/png" or "audio/mpeg".
	Put(ctx context.Context, sha256 string, data []byte, contentType string) error
}
