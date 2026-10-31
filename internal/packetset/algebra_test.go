package packetset

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func newTestBudget(t *testing.T) *Budget {
	t.Helper()
	b, err := NewBudget(context.Background(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func cubeMatch(c Cube, x uint32) bool {
	for bit := uint(0); bit < 32; bit++ {
		if c.Wildcard&(uint32(1)<<bit) == 0 && (c.Value>>bit)&1 != (x>>bit)&1 {
			return false
		}
	}
	return true
}

func oracle(b Box, p uint8, src, dst uint32, sp, dp uint16) bool {
	return b.Protocols[p/64]&(uint64(1)<<(p%64)) != 0 && cubeMatch(b.Src, src) && cubeMatch(b.Dst, dst) && sp >= b.SrcPort.Lo && sp <= b.SrcPort.Hi && dp >= b.DstPort.Lo && dp <= b.DstPort.Hi
}

func count(r Region, p uint8, src, dst uint32, sp, dp uint16) int {
	n := 0
	for _, b := range r {
		if oracle(b, p, src, dst, sp, dp) {
			n++
		}
	}
	return n
}

func cubes(width uint) []Cube {
	out := []Cube{}
	for mask := uint32(0); mask < 1<<width; mask++ {
		for value := uint32(0); value < 1<<width; value++ {
			if value&mask == 0 {
				out = append(out, Cube{Value: value, Wildcard: mask})
			}
		}
	}
	return out
}

func sourceBox(c Cube) Box {
	return Box{Protocols: Protocol(6), Src: c, Dst: Cube{}, SrcPort: Interval{0, 65535}, DstPort: Interval{0, 65535}}
}

func TestExhaustiveFourBitCubeDifferenceAndIntersection(t *testing.T) {
	cs := cubes(4)
	for _, a := range cs {
		for _, b := range cs {
			budget, err := NewBudget(context.Background(), Defaults())
			if err != nil {
				t.Fatal(err)
			}
			diff, err := Difference(Region{sourceBox(a)}, Region{sourceBox(b)}, budget)
			if err != nil {
				t.Fatal(err)
			}
			i, exists := Intersect(sourceBox(a), sourceBox(b))
			for x := uint32(0); x < 16; x++ {
				want := cubeMatch(a, x) && !cubeMatch(b, x)
				n := count(diff, 6, x, 0, 0, 0)
				if (n == 1) != want || n > 1 {
					t.Fatalf("cube diff a=%+v b=%+v x=%d got=%d want=%v", a, b, x, n, want)
				}
				if (exists && oracle(i, 6, x, 0, 0, 0)) != (cubeMatch(a, x) && cubeMatch(b, x)) {
					t.Fatalf("cube intersection wrong a=%+v b=%+v x=%d", a, b, x)
				}
			}
			budget.Close()
		}
	}
}

func randomBox(r *rand.Rand, cs []Cube) Box {
	lo, hi := uint16(r.Intn(4)), uint16(r.Intn(4))
	if lo > hi {
		lo, hi = hi, lo
	}
	dlo, dhi := uint16(r.Intn(4)), uint16(r.Intn(4))
	if dlo > dhi {
		dlo, dhi = dhi, dlo
	}
	var ps Protocols
	for ps == (Protocols{}) {
		for _, p := range []uint8{0, 6, 17} {
			if r.Intn(2) == 1 {
				ps[p/64] |= uint64(1) << (p % 64)
			}
		}
	}
	return Box{Protocols: ps, Src: cs[r.Intn(len(cs))], Dst: cs[r.Intn(len(cs))], SrcPort: Interval{lo, hi}, DstPort: Interval{dlo, dhi}}
}

func TestFiveDimensionAlgebraAgainstReducedDomain(t *testing.T) {
	rng := rand.New(rand.NewSource(20261030))
	cs := cubes(3)
	for trial := 0; trial < 100; trial++ {
		a, b := randomBox(rng, cs), randomBox(rng, cs)
		budget := newTestBudget(t)
		diff, err := Difference(Region{a}, Region{b}, budget)
		if err != nil {
			t.Fatal(err)
		}
		inter, err := Intersection(Region{a}, Region{b}, budget)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []uint8{0, 6, 17} {
			for src := uint32(0); src < 8; src++ {
				for dst := uint32(0); dst < 8; dst++ {
					for sp := uint16(0); sp < 4; sp++ {
						for dp := uint16(0); dp < 4; dp++ {
							inA, inB := oracle(a, p, src, dst, sp, dp), oracle(b, p, src, dst, sp, dp)
							nd, ni := count(diff, p, src, dst, sp, dp), count(inter, p, src, dst, sp, dp)
							if nd > 1 || ni > 1 || (nd == 1) != (inA && !inB) || (ni == 1) != (inA && inB) {
								t.Fatalf("trial%d tuple%d/%d/%d/%d/%d wrong diff=%d intersection=%d", trial, p, src, dst, sp, dp, nd, ni)
							}
							if Contains(a, p, src, dst, sp, dp) != inA {
								t.Fatal("Contains differs from direct-bit oracle")
							}
						}
					}
				}
			}
		}
	}
}

func TestCuttersAreUnionEvenWhenOverlappingOrDuplicated(t *testing.T) {
	a := sourceBox(Cube{Wildcard: 15})
	b := sourceBox(Cube{Value: 0, Wildcard: 7})
	c := sourceBox(Cube{Value: 4, Wildcard: 11})
	before := Region{a}
	cutters := Region{b, c, b}
	acopy, bcopy := append(Region{}, before...), append(Region{}, cutters...)
	out, err := Difference(before, cutters, newTestBudget(t))
	if err != nil {
		t.Fatal(err)
	}
	for x := uint32(0); x < 16; x++ {
		want := oracle(a, 6, x, 0, 0, 0) && !(oracle(b, 6, x, 0, 0, 0) || oracle(c, 6, x, 0, 0, 0))
		n := count(out, 6, x, 0, 0, 0)
		if n > 1 || (n == 1) != want {
			t.Fatalf("union subtraction wrong at%d", x)
		}
	}
	if !reflect.DeepEqual(before, acopy) || !reflect.DeepEqual(cutters, bcopy) {
		t.Fatal("algebra mutated caller regions")
	}
	copyOnly, err := Difference(before, nil, newTestBudget(t))
	if err != nil {
		t.Fatal(err)
	}
	copyOnly[0].Src.Value = 99
	if before[0].Src.Value == 99 {
		t.Fatal("empty cutter result aliases caller region")
	}
}

func TestPortBoundariesWitnessAndSort(t *testing.T) {
	a := sourceBox(Cube{})
	for _, point := range []uint16{0, 65535} {
		cut := a
		cut.DstPort = Interval{point, point}
		diff, err := Difference(Region{a}, Region{cut}, newTestBudget(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []uint16{0, 1, 65534, 65535} {
			if (count(diff, 6, 0, 0, 0, p) > 0) != (p != point) {
				t.Fatalf("port edge wrong cut=%d point=%d", point, p)
			}
		}
	}
	nonTCP := a
	nonTCP.Protocols = Protocol(47)
	p, src, dst, sp, dp, ok := nonTCP.Witness()
	if !ok || p != 47 || sp != 0 || dp != 0 || !Contains(nonTCP, p, src, dst, sp, dp) {
		t.Fatal("invalid nonTCP witness")
	}
	nonTCP.DstPort = Interval{22, 22}
	if _, _, _, _, _, ok := nonTCP.Witness(); ok {
		t.Fatal("invented nonTCP port witness")
	}
	x := a
	x.DstPort = Interval{22, 22}
	y := a
	y.DstPort = Interval{80, 80}
	r := Region{y, x}
	Sort(r)
	once := append(Region{}, r...)
	Sort(r)
	if !reflect.DeepEqual(r, once) || r[0].DstPort.Lo != 22 {
		t.Fatal("sort not stable/canonical")
	}
	low, high := a, a
	low.Protocols = Protocol(0)
	high.Protocols = Protocol(64)
	r = Region{high, low}
	Sort(r)
	if r[0].Protocols != Protocol(0) {
		t.Fatal("protocol sort must start with lowest member")
	}
}

func TestInvalidBoxesNeverReturnPartialResult(t *testing.T) {
	a := sourceBox(Cube{})
	bad := a
	bad.Src = Cube{Value: 1, Wildcard: 1}
	for _, op := range []func(Region, Region, *Budget) (Region, error){Difference, Intersection} {
		got, err := op(Region{a, bad}, Region{a}, newTestBudget(t))
		if !errors.Is(err, ErrInvalid) || got != nil {
			t.Fatal("malformed box accepted or partial result returned")
		}
	}
}

func TestLogicalBudgetsCancelAndControlledDeadline(t *testing.T) {
	a := sourceBox(Cube{Wildcard: 15})
	b := sourceBox(Cube{})
	limits := Defaults()
	limits.MaxOperations = 1
	budget, err := NewBudget(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Difference(Region{a}, Region{b}, budget)
	if !errors.Is(err, ErrLimit) || got != nil || budget.Stats().StopReason != "operations" || budget.Stats().TimingLimited {
		t.Fatal("logical operation cap not honored")
	}
	limits = Defaults()
	limits.MaxBoxes = 2
	budget, err = NewBudget(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Difference(Region{a}, Region{b}, budget); !errors.Is(err, ErrLimit) || budget.Stats().StopReason != "boxes" {
		t.Fatal("live workspace box cap ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	budget, err = NewBudget(ctx, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := budget.Tick(1); !errors.Is(err, context.Canceled) || budget.Stats().TimingLimited {
		t.Fatal("cancel conflated with deadline")
	}
	budget = newTestBudget(t)
	budget.now = func() time.Time { return budget.deadline }
	if err := budget.Tick(1); !errors.Is(err, ErrLimit) || !budget.Stats().TimingLimited || budget.Stats().StopReason != "wall_clock" {
		t.Fatal("controlled deadline not honored")
	}
	if _, err := NewBudget(context.Background(), Limits{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid limits accepted")
	}
}
