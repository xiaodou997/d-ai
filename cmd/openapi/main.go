// Command openapi exports the single code-first contract used by the D-AI
// backend and Portal. It deliberately registers routes with empty runtime
// dependencies: handlers are never invoked during export, so no database or
// Redis connection is needed to produce the contract.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/transport"
	"xiaodou/dai/libs/go/server"
)

const contractPath = "contracts/openapi.yaml"
const contractVersion = "dev"

func main() {
	_, api := server.New(server.Options{
		Title:   "D-AI",
		Version: contractVersion,
	})
	transport.Register(api, transport.Deps{
		Version: contractVersion,
	})
	// Runtime gateway endpoints are native chi routes, so their schemas are
	// added explicitly to the same Huma document.
	gateway.RegisterOpenAPI(api)

	contract, err := server.OpenAPIYAML(api)
	if err != nil {
		fail("export OpenAPI contract", err)
	}
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		fail("create contract directory", err)
	}
	if err := os.WriteFile(contractPath, contract, 0o644); err != nil {
		fail("write OpenAPI contract", err)
	}
	fmt.Printf("OpenAPI contract written to %s (%d bytes)\n", contractPath, len(contract))
}

func fail(action string, err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	fmt.Fprintf(os.Stderr, "openapi: %s: %v\n", action, err)
	os.Exit(1)
}
