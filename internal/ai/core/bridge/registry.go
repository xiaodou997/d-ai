package bridge

import "xiaodou/dai/internal/ai/core/surface"

// Registry stores the bridge definitions supported by a runtime kernel
// instance. It is intentionally light-weight: the concrete adapters still own
// the actual conversion logic, while the registry provides explicit
// capability/surface wiring for validation and dispatch.
type Registry struct {
	definitions []Definition
}

// NewRegistry builds a registry from the supplied bridge definitions.
func NewRegistry(definitions ...Definition) *Registry {
	r := &Registry{}
	r.Register(definitions...)
	return r
}

// Register appends bridge definitions, ignoring duplicates.
func (r *Registry) Register(definitions ...Definition) {
	if r == nil {
		return
	}
	for _, def := range definitions {
		if r.Supports(def.Kind, def.Source, def.Target) {
			continue
		}
		r.definitions = append(r.definitions, def)
	}
}

// Supports reports whether the registry contains a bridge definition matching
// the supplied IR kind and source/target surfaces.
func (r *Registry) Supports(kind IRKind, source, target surface.ID) bool {
	if r == nil {
		return false
	}
	for _, def := range r.definitions {
		if def.Kind == kind && def.Source == source && def.Target == target {
			return true
		}
	}
	return false
}

// Definitions returns a copy of the registered definitions for inspection.
func (r *Registry) Definitions() []Definition {
	if r == nil || len(r.definitions) == 0 {
		return nil
	}
	out := make([]Definition, len(r.definitions))
	copy(out, r.definitions)
	return out
}

// Matches reports whether this definition matches the supplied bridge tuple.
func (d Definition) Matches(kind IRKind, source, target surface.ID) bool {
	return d.Kind == kind && d.Source == source && d.Target == target
}
