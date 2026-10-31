package packetset

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReservedRegionsCountAlongsideWorkspaceAndCanBeReleased(t *testing.T) {
	limits := Defaults()
	limits.MaxBoxes = 4
	b, err := NewBudget(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.SetReserved(2); err != nil {
		t.Fatal(err)
	}
	if err := b.CheckBoxes(2); err != nil {
		t.Fatal(err)
	}
	if b.Stats().PeakBoxes != 4 {
		t.Fatal("reserved regions missing from peak")
	}
	if err := b.SetReserved(0); err != nil {
		t.Fatal(err)
	}
	if err := b.CheckBoxes(4); err != nil {
		t.Fatal("released reservation still consumes capacity")
	}
	if err := b.SetReserved(3); err != nil {
		t.Fatal(err)
	}
	if err := b.CheckBoxes(2); !errors.Is(err, ErrLimit) || b.Stats().StopReason != "boxes" {
		t.Fatal("aggregate cap ignored")
	}
}

func TestInjectedClockForcesDeadlineWithoutWaiting(t *testing.T) {
	current := time.Unix(1_000, 0)
	b, err := NewBudgetWithClock(context.Background(), Defaults(), func() time.Time { return current })
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Tick(1); err != nil {
		t.Fatal(err)
	}
	current = current.Add(Defaults().Timeout)
	if err := b.Tick(1); !errors.Is(err, ErrLimit) || !b.Stats().TimingLimited {
		t.Fatal("injected deadline ignored")
	}
	if _, err := NewBudgetWithClock(context.Background(), Defaults(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil clock accepted")
	}
}

func TestReservationLimitsAvoidIntegerOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, reserve := range []bool{false, true} {
		b := newTestBudget(t)
		if err := b.SetReserved(1); err != nil {
			t.Fatal(err)
		}
		var err error
		if reserve {
			err = b.SetReserved(maxInt)
		} else {
			err = b.CheckBoxes(maxInt)
		}
		if !errors.Is(err, ErrLimit) || b.Stats().StopReason != "boxes" {
			t.Fatal("overflow-sized count bypassed cap")
		}
	}
	b := newTestBudget(t)
	if err := b.SetReserved(-1); !errors.Is(err, ErrInvalid) {
		t.Fatal("negative reservation accepted")
	}
	if err := b.SetReserved(0); err != nil {
		t.Fatal("invalid request mutated reservation or stopped budget")
	}
}
