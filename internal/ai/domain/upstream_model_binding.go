package domain

import "time"

type UpstreamKind string

const (
	UpstreamKindDirect UpstreamKind = "direct_upstream"
	UpstreamKindPool   UpstreamKind = "oauth_pool"
)

type UpstreamModelBindingScope struct {
	Kind UpstreamKind
	ID   string
}

type UpstreamModelBinding struct {
	ID                string
	ModelCode         string
	CapabilityType    string
	APIFormat         string
	UpstreamModelName string
	Status            string
	ConfigJSON        []byte
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UpstreamModelBindingWrite struct {
	ModelCode         string
	CapabilityType    string
	APIFormat         string
	UpstreamModelName string
	Status            string
	ConfigJSON        []byte
}

type UpstreamModelBindingImportResult struct {
	Created []string
	Skipped []string
}
