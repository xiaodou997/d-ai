package main

import "testing"

func TestDependencyRulesRejectNewTransportPersistenceEdge(t *testing.T) {
	rules := dependencyRules()
	for _, rule := range rules {
		if rule.Name != "transport-must-not-own-persistence" {
			continue
		}
		if !rule.Match(modulePrefix+"transport", modulePrefix+"billing/pg") {
			t.Fatal("transport -> billing/pg must be rejected")
		}
		if rule.Match(modulePrefix+"billing/service", modulePrefix+"billing/pg") {
			t.Fatal("application package must not be classified as transport")
		}
		return
	}
	t.Fatal("transport persistence rule not registered")
}

func TestDependencyRulesRejectInfrastructureFromInnerLayers(t *testing.T) {
	for _, rule := range dependencyRules() {
		if rule.Name != "inner-layers-must-not-import-infrastructure" {
			continue
		}
		if !rule.Match(modulePrefix+"catalog/domain", "github.com/jackc/pgx/v5/pgxpool") {
			t.Fatal("domain -> pgxpool must be rejected")
		}
		if rule.Match(modulePrefix+"catalog/application", modulePrefix+"catalog/domain") {
			t.Fatal("application -> domain should remain allowed")
		}
		return
	}
	t.Fatal("inner-layer rule not registered")
}
