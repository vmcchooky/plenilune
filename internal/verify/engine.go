package verify

import (
	"errors"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

const (
	// EngineVersion is the engine stamped on new evidence. v0.3 aligns the
	// semantically identical rule prefix of the two ACLs and computes the
	// exact delta only over the residual rules, on the complement of the
	// prefix's covered region; verdicts, change classes and obligation
	// outcomes equal v0.2, but logical operation and box accounting differ,
	// so the version is part of evidence.
	EngineVersion = "acl-exact/v0.3"
	// IndexedEngineVersion is kept so v0.2 receipts replay with their
	// original accounting instead of being silently invalidated.
	IndexedEngineVersion = "acl-exact/v0.2"
	// LegacyEngineVersion is kept so v0.1 receipts replay with their original
	// accounting instead of being silently invalidated.
	LegacyEngineVersion = "acl-exact/v0.1"
)

type engine struct{ indexed, lcp bool }

func engineFor(version string) (engine, error) {
	switch version {
	case EngineVersion:
		return engine{indexed: true, lcp: true}, nil
	case IndexedEngineVersion:
		return engine{indexed: true}, nil
	case LegacyEngineVersion:
		return engine{}, nil
	}
	return engine{}, errors.New("unsupported engine version")
}

// SupportedEngine reports whether evidence produced by version can be replayed.
func SupportedEngine(version string) bool {
	_, err := engineFor(version)
	return err == nil
}

// delta computes the exact behavior difference with the accounting of the
// engine version: v0.3 uses the LCP-residual construction, v0.2 the full
// indexed construction. Callers on the legacy path never reach this.
func (e engine) delta(before, after ciscoacl.ACL, b *packetset.Budget) (delta, error) {
	if e.lcp {
		return exactDeltaResidual(before, after, b)
	}
	return exactDeltaV2(before, after, b)
}

func (e engine) difference(a, b packetset.Region, budget *packetset.Budget) (packetset.Region, error) {
	if !e.indexed {
		return packetset.Difference(a, b, budget)
	}
	idx, err := packetset.IndexOf(b, budget)
	if err != nil {
		return nil, err
	}
	return packetset.DifferenceIndexed(a, idx, budget)
}

func (e engine) intersection(a, b packetset.Region, budget *packetset.Budget) (packetset.Region, error) {
	if !e.indexed {
		return packetset.Intersection(a, b, budget)
	}
	idx, err := packetset.IndexOf(b, budget)
	if err != nil {
		return nil, err
	}
	return packetset.IntersectionIndexed(a, idx, budget)
}
