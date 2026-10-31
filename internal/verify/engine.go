package verify

import (
	"errors"
	"plenilune/internal/packetset"
)

const (
	// EngineVersion is the engine stamped on new evidence. v0.2 visits only
	// index candidates; regions, witnesses and verdicts equal v0.1, but logical
	// operation and box accounting differ, so the version is part of evidence.
	EngineVersion = "acl-exact/v0.2"
	// LegacyEngineVersion is kept so v0.1 receipts replay with their original
	// accounting instead of being silently invalidated.
	LegacyEngineVersion = "acl-exact/v0.1"
)

type engine struct{ indexed bool }

func engineFor(version string) (engine, error) {
	switch version {
	case EngineVersion:
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
