package verify

import (
	"context"
	"errors"
	"net/netip"
	"plenilune/internal/intent"
	"testing"
)

func allowed(src string) intent.Region {
	return intent.Region{Protocol: "tcp", SrcCIDR: src, DstCIDR: "198.51.100.10/32", DstPorts: []intent.Interval{{Lo: 22, Hi: 22}}}
}

func TestCollateralChecksWholeRegionNotRepresentativePackets(t *testing.T) {
	b := parse(t, " deny ip any any\n")
	c := parse(t, " permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n")
	r, err := PreserveOutside(context.Background(), b, c, []intent.Region{allowed("192.0.2.0/25")}, DefaultOptions())
	if err != nil || r.Status != "FAILED" || r.Witness == nil {
		t.Fatalf("collateral: %v %+v", err, r)
	}
	if netip.MustParsePrefix("192.0.2.0/25").Contains(r.Witness.Packet.SrcIP) {
		t.Fatal("only canonical in-allowed packet was checked")
	}
	r, err = PreserveOutside(context.Background(), b, c, []intent.Region{allowed("192.0.2.0/25"), allowed("192.0.2.128/25")}, DefaultOptions())
	if err != nil || r.Status != "VERIFIED" || r.Witness != nil {
		t.Fatalf("union regions must cover all changes: %v %+v", err, r)
	}
}

func TestCollateralEmptyUnionExcludesAndDomain(t *testing.T) {
	b := parse(t, " deny ip any any\n")
	c := parse(t, " permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n")
	region := allowed("192.0.2.0/24")
	region.ExcludeSrc = []string{"192.0.2.5/32"}
	r, err := PreserveOutside(context.Background(), b, c, []intent.Region{region}, DefaultOptions())
	if err != nil || r.Status != "FAILED" || r.Witness.Packet.SrcIP.String() != "192.0.2.5" {
		t.Fatalf("exclude ignored: %v %+v", err, r)
	}
	r, err = PreserveOutside(context.Background(), b, b, []intent.Region{}, DefaultOptions())
	if err != nil || r.Status != "VERIFIED" {
		t.Fatal("unchanged ACL with zero allowed changes must pass")
	}
	r, err = PreserveOutside(context.Background(), b, c, []intent.Region{}, DefaultOptions())
	if err != nil || r.Status != "FAILED" {
		t.Fatal("empty allowed union permitted change")
	}
	c = parse(t, " permit ip any any\n")
	r, err = PreserveOutside(context.Background(), b, c, []intent.Region{allowed("192.0.2.0/24")}, DefaultOptions())
	if err != nil || r.Status != "FAILED" || r.Witness.Packet.Protocol != 0 || r.Witness.Packet.SrcPort != 0 || r.Witness.Packet.DstPort != 0 {
		t.Fatalf("non-TCP domain escaped collateral: %v %+v", err, r)
	}
	opts := DefaultOptions()
	opts.Limits.MaxOperations = 1
	r, err = PreserveOutside(context.Background(), b, c, nil, opts)
	if err != nil || r.Status != "INCONCLUSIVE" {
		t.Fatal("budget exhaustion became proof")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PreserveOutside(ctx, b, c, nil, DefaultOptions()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
}
