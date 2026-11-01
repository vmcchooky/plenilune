package verify

import (
	"errors"

	"plenilune/internal/ciscoacl"
	"plenilune/internal/packetset"
)

const (
	// EngineVersion is the engine stamped on new evidence. v0.4 is the
	// v0.3 LCP-residual construction plus partial-result reporting: when
	// the budget stops the search, the result records which rule pairs
	// were already decided and bounds the undecided remainder (P1-2).
	// Completing runs produce the same verdicts, change classes,
	// obligation outcomes and budget accounting as v0.3.
	EngineVersion = "acl-exact/v0.4"
	// ResidualEngineVersion is the v0.3 LCP-residual engine, kept so
	// v0.3 receipts replay with their original all-or-nothing shape.
	ResidualEngineVersion = "acl-exact/v0.3"
	// IndexedEngineVersion is kept so v0.2 receipts replay with their
	// original accounting instead of being silently invalidated.
	IndexedEngineVersion = "acl-exact/v0.2"
	// LegacyEngineVersion is kept so v0.1 receipts replay with their original
	// accounting instead of being silently invalidated.
	LegacyEngineVersion = "acl-exact/v0.1"
)

type engine struct{ indexed, lcp, partial bool }

func engineFor(version string) (engine, error) {
	switch version {
	case EngineVersion:
		return engine{indexed: true, lcp: true, partial: true}, nil
	case ResidualEngineVersion:
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
// engine version: v0.3/v0.4 use the LCP-residual construction, v0.2 the full
// indexed construction. Callers on the legacy path never reach this.
func (e engine) delta(before, after ciscoacl.ACL, b *packetset.Budget) (delta, error) {
	if e.lcp {
		return exactDeltaResidual(before, after, b)
	}
	return exactDeltaV2(before, after, b)
}

// deltaPartial is the partial-reporting delta of engine v0.4: the same
// LCP-residual construction and the same budget accounting as delta, but
// a budget stop returns the progress made (deltaProgress) and the pieces
// decided before the stop instead of discarding them. Only engines with
// partial set reach this; the progress is nil for all other engines.
func (e engine) deltaPartial(before, after ciscoacl.ACL, b *packetset.Budget) (delta, *deltaProgress, error) {
	if !e.partial {
		d, err := e.delta(before, after, b)
		return d, nil, err
	}
	return exactDeltaResidualPartial(before, after, b)
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
