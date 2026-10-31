// Package intent validates operator-visible conditions without interpreting prose.
package intent

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"plenilune/internal/jsonio"
	"regexp"
	"slices"
	"strconv"
)

const Schema = "plenilune.obligations/v1"
const Scope = "ipv4-unfragmented-or-initial-with-complete-l4/v1"
const MaxJSONBytes = 64 << 10

type Set struct {
	Schema      string       `json:"schema"`
	Scope       string       `json:"scope"`
	Obligations []Obligation `json:"obligations"`
}
type Obligation struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Match Region `json:"match"`
}
type Region struct {
	Protocol   string     `json:"protocol"`
	SrcCIDR    string     `json:"src_cidr"`
	DstCIDR    string     `json:"dst_cidr"`
	ExcludeSrc []string   `json:"exclude_src"`
	ExcludeDst []string   `json:"exclude_dst"`
	SrcPorts   []Interval `json:"src_ports"`
	DstPorts   []Interval `json:"dst_ports"`
}
type Interval struct {
	Lo uint16 `json:"lo"`
	Hi uint16 `json:"hi"`
}

func (p *Interval) UnmarshalJSON(data []byte) error {
	var in struct {
		Lo *uint16 `json:"lo"`
		Hi *uint16 `json:"hi"`
	}
	if err := jsonio.Decode(data, 256, &in); err != nil {
		return err
	}
	if in.Lo == nil || in.Hi == nil {
		return jsonio.Invalid("port interval requires lo and hi")
	}
	p.Lo, p.Hi = *in.Lo, *in.Hi
	return nil
}

var obligationID = regexp.MustCompile(`^OB-[1-9][0-9]?$`)

// Normalize creates an owned canonical value. It never proves that the
// conditions capture the operator's natural-language intent or hold on an ACL.
func Normalize(in Set) (Set, error) {
	if in.Schema != Schema || in.Scope != Scope {
		return Set{}, errors.New("unsupported obligation schema or scope")
	}
	if len(in.Obligations) == 0 || len(in.Obligations) > 12 {
		return Set{}, errors.New("expected 1 to 12 obligations")
	}
	out := Set{Schema: Schema, Scope: Scope, Obligations: make([]Obligation, len(in.Obligations))}
	seen := make(map[string]bool)
	for i, ob := range in.Obligations {
		if !obligationID.MatchString(ob.ID) || seen[ob.ID] {
			return Set{}, errors.New("obligation IDs must be valid and unique")
		}
		seen[ob.ID] = true
		if ob.Kind != "must_permit" && ob.Kind != "must_deny" && ob.Kind != "must_remain" {
			return Set{}, errors.New("unsupported obligation kind")
		}
		r, err := normalizeRegion(ob.Match)
		if err != nil {
			return Set{}, err
		}
		out.Obligations[i] = Obligation{ID: ob.ID, Kind: ob.Kind, Match: r}
	}
	slices.SortFunc(out.Obligations, func(a, b Obligation) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// Digest binds accepted predicate definitions and scope, not just result labels.
func Digest(in Set) (Set, string, error) {
	n, err := Normalize(in)
	if err != nil {
		return Set{}, "", err
	}
	b, err := json.Marshal(n)
	if err != nil {
		return Set{}, "", err
	}
	sum := sha256.Sum256(b)
	return n, hex.EncodeToString(sum[:]), nil
}

func normalizeRegion(in Region) (Region, error) {
	out := in
	switch in.Protocol {
	case "ip", "tcp", "udp", "icmp":
	default:
		n, err := strconv.ParseUint(in.Protocol, 10, 8)
		if err != nil {
			return Region{}, errors.New("unsupported obligation protocol")
		}
		out.Protocol = strconv.FormatUint(n, 10)
		switch n {
		case 6:
			out.Protocol = "tcp"
		case 17:
			out.Protocol = "udp"
		case 1:
			out.Protocol = "icmp"
		}
	}
	var err error
	out.SrcCIDR, out.ExcludeSrc, err = prefixes(in.SrcCIDR, in.ExcludeSrc)
	if err != nil {
		return Region{}, err
	}
	out.DstCIDR, out.ExcludeDst, err = prefixes(in.DstCIDR, in.ExcludeDst)
	if err != nil {
		return Region{}, err
	}
	if out.Protocol != "tcp" && out.Protocol != "udp" {
		if len(in.SrcPorts) != 0 || len(in.DstPorts) != 0 {
			return Region{}, errors.New("port predicates require tcp or udp")
		}
		out.SrcPorts, out.DstPorts = []Interval{}, []Interval{}
		return out, nil
	}
	out.SrcPorts, err = intervals(in.SrcPorts)
	if err != nil {
		return Region{}, err
	}
	out.DstPorts, err = intervals(in.DstPorts)
	return out, err
}

func prefix(text string) (netip.Prefix, error) {
	if text == "any" {
		text = "0.0.0.0/0"
	}
	p, err := netip.ParsePrefix(text)
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, errors.New("expected IPv4 CIDR or any")
	}
	return p.Masked(), nil
}

func prefixes(include string, excludes []string) (string, []string, error) {
	p, err := prefix(include)
	if err != nil {
		return "", nil, err
	}
	if len(excludes) > 16 {
		return "", nil, errors.New("too many excluded prefixes")
	}
	ps := make([]netip.Prefix, 0, len(excludes))
	for _, text := range excludes {
		x, err := prefix(text)
		if err != nil {
			return "", nil, err
		}
		if x.Bits() < p.Bits() || !p.Contains(x.Addr()) {
			return "", nil, errors.New("exclude must be within include")
		}
		ps = append(ps, x)
	}
	slices.SortFunc(ps, func(a, b netip.Prefix) int {
		if a.Bits() != b.Bits() {
			return a.Bits() - b.Bits()
		}
		return a.Addr().Compare(b.Addr())
	})
	kept := make([]netip.Prefix, 0, len(ps))
	var removed uint64
	for _, x := range ps {
		covered := false
		for _, k := range kept {
			if k.Contains(x.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, x)
			removed += uint64(1) << (32 - x.Bits())
		}
	}
	if removed == uint64(1)<<(32-p.Bits()) {
		return "", nil, errors.New("excluded prefixes leave an empty region")
	}
	out := make([]string, len(kept))
	for i, x := range kept {
		out[i] = x.String()
	}
	slices.Sort(out)
	return p.String(), out, nil
}

func intervals(in []Interval) ([]Interval, error) {
	if len(in) > 16 {
		return nil, errors.New("too many port intervals")
	}
	if len(in) == 0 {
		return []Interval{{Lo: 0, Hi: 65535}}, nil
	}
	xs := slices.Clone(in)
	for _, x := range xs {
		if x.Lo > x.Hi {
			return nil, errors.New("invalid port interval")
		}
	}
	slices.SortFunc(xs, func(a, b Interval) int {
		if a.Lo != b.Lo {
			return int(a.Lo) - int(b.Lo)
		}
		return int(a.Hi) - int(b.Hi)
	})
	out := make([]Interval, 0, len(xs))
	for _, x := range xs {
		if len(out) > 0 && uint32(x.Lo) <= uint32(out[len(out)-1].Hi)+1 {
			if x.Hi > out[len(out)-1].Hi {
				out[len(out)-1].Hi = x.Hi
			}
		} else {
			out = append(out, x)
		}
	}
	return out, nil
}
