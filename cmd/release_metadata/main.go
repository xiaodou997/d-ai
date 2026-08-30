// Command release_metadata creates the machine-readable release evidence
// consumed by the production release gate: an SPDX SBOM and a provenance
// record. Dependencies are resolved from the checked-in manifests/lockfiles
// and the frozen Bun install prepared by the release build.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type goModule struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Main    bool   `json:"Main"`
	Replace *struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	} `json:"Replace"`
}

type spdxPackage struct {
	SPDXID           string `json:"SPDXID"`
	Name             string `json:"name"`
	VersionInfo      string `json:"versionInfo"`
	DownloadLocation string `json:"downloadLocation"`
	FilesAnalyzed    bool   `json:"filesAnalyzed"`
	LicenseConcluded string `json:"licenseConcluded"`
	LicenseDeclared  string `json:"licenseDeclared"`
	CopyrightText    string `json:"copyrightText"`
}

type spdxDocument struct {
	SPDXVersion       string         `json:"spdxVersion"`
	SPDXID            string         `json:"SPDXID"`
	Name              string         `json:"name"`
	DataLicense       string         `json:"dataLicense"`
	DocumentNamespace string         `json:"documentNamespace"`
	CreationInfo      map[string]any `json:"creationInfo"`
	Packages          []spdxPackage  `json:"packages"`
}

func main() {
	outDir := flag.String("out", "release", "release output directory")
	version := flag.String("version", "dev", "release version")
	flag.Parse()

	if err := run(*outDir, *version); err != nil {
		fmt.Fprintf(os.Stderr, "release metadata: %v\n", err)
		os.Exit(1)
	}
}

func run(outDir, version string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	modules, err := goModules()
	if err != nil {
		return fmt.Errorf("list Go modules: %w", err)
	}
	packages, err := npmPackages()
	if err != nil {
		return fmt.Errorf("read Bun lockfile: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	spdx := spdxDocument{
		SPDXVersion:       "SPDX-2.3",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              "dai-" + version,
		DataLicense:       "CC0-1.0",
		DocumentNamespace: "https://xiaodou.dev/dai/releases/" + sanitize(version) + "/" + strings.ReplaceAll(now, ":", "-"),
		CreationInfo: map[string]any{
			"created":  now,
			"creators": []string{"Tool: xiaodou/dai/cmd/release_metadata"},
		},
		Packages: append(modules, packages...),
	}
	sort.Slice(spdx.Packages, func(i, j int) bool { return spdx.Packages[i].Name < spdx.Packages[j].Name })
	if err := writeJSON(filepath.Join(outDir, "SBOM.spdx.json"), spdx); err != nil {
		return fmt.Errorf("write SBOM: %w", err)
	}

	gitRevision := commandOutput("git", "rev-parse", "HEAD")
	gitStatus := commandOutput("git", "status", "--porcelain")
	goVersion := commandOutput("go", "version")
	bunVersion := commandOutput("bun", "--version")
	provenance := map[string]any{
		"schema":       "dai.release.provenance.v1",
		"version":      version,
		"created":      now,
		"git_revision": gitRevision,
		"git_dirty":    strings.TrimSpace(gitStatus) != "",
		"go_version":   goVersion,
		"bun_version":  bunVersion,
		"inputs":       []string{"go.mod", "go.sum", "package.json", "bun.lock"},
		"sbom":         "SBOM.spdx.json",
	}
	if err := writeJSON(filepath.Join(outDir, "PROVENANCE.json"), provenance); err != nil {
		return fmt.Errorf("write provenance: %w", err)
	}
	return nil
}

func goModules() ([]spdxPackage, error) {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	packages := make([]spdxPackage, 0)
	for {
		var module goModule
		if err := decoder.Decode(&module); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if module.Path == "" {
			continue
		}
		version := module.Version
		path := module.Path
		if module.Replace != nil {
			path = module.Replace.Path
			if module.Replace.Version != "" {
				version = module.Replace.Version
			}
		}
		if version == "" {
			version = "(local)"
		}
		packages = append(packages, spdxPackage{
			SPDXID:           "SPDXRef-Go-" + sanitize(module.Path),
			Name:             module.Path,
			VersionInfo:      version,
			DownloadLocation: goModuleDownloadLocation(module, path, version),
			FilesAnalyzed:    false,
			LicenseConcluded: "NOASSERTION",
			LicenseDeclared:  "NOASSERTION",
			CopyrightText:    "NOASSERTION",
		})
	}
	return packages, nil
}

func goModuleDownloadLocation(module goModule, path, version string) string {
	if module.Main || version == "(local)" {
		return "NOASSERTION"
	}
	return "https://proxy.golang.org/" + path + "/@v/" + version + ".mod"
}

func npmPackages() ([]spdxPackage, error) {
	output, err := exec.Command("bun", "pm", "licenses", "--json").Output()
	if err != nil {
		return nil, err
	}
	var licenses map[string][]struct {
		Name     string   `json:"name"`
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(output, &licenses); err != nil {
		return nil, err
	}
	type packageRef struct {
		name    string
		version string
		license string
	}
	refs := make([]packageRef, 0)
	for license, entries := range licenses {
		for _, entry := range entries {
			for _, version := range entry.Versions {
				refs = append(refs, packageRef{name: entry.Name, version: version, license: license})
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].name == refs[j].name {
			if refs[i].version == refs[j].version {
				return refs[i].license < refs[j].license
			}
			return refs[i].version < refs[j].version
		}
		return refs[i].name < refs[j].name
	})
	packages := make([]spdxPackage, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key := ref.name + "@" + ref.version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		packages = append(packages, spdxPackage{
			SPDXID:           "SPDXRef-NPM-" + sanitize(key),
			Name:             ref.name,
			VersionInfo:      ref.version,
			DownloadLocation: "NOASSERTION",
			FilesAnalyzed:    false,
			LicenseConcluded: "NOASSERTION",
			LicenseDeclared:  spdxLicense(ref.license),
			CopyrightText:    "NOASSERTION",
		})
	}
	return packages, nil
}

func spdxLicense(value string) string {
	if strings.TrimSpace(value) == "" {
		return "NOASSERTION"
	}
	return value
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func commandOutput(name string, args ...string) string {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(output))
}

func sanitize(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('-')
		}
	}
	return builder.String()
}
