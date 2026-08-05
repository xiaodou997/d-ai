// Package migrations embeds SQL migration files into the binary.
// This eliminates the need for an external migrations directory at runtime,
// enabling true single-binary deployment in Docker/Kubernetes environments.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
