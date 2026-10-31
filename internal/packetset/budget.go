// Package packetset implements exact, bounded algebra over packet header boxes.
package packetset

import (
	"context"
	"errors"
	"time"
)

var (
	ErrLimit   = errors.New("packet-set budget exhausted")
	ErrInvalid = errors.New("invalid packet-set input")
)

type Limits struct {
	MaxOperations int           `json:"max_operations"`
	MaxBoxes      int           `json:"max_boxes"`
	Timeout       time.Duration `json:"timeout_ns"`
}

func Defaults() Limits {
	return Limits{MaxOperations: 2_000_000, MaxBoxes: 50_000, Timeout: 2 * time.Second}
}

type Stats struct {
	Operations    int    `json:"operations"`
	PeakBoxes     int    `json:"peak_boxes"`
	StopReason    string `json:"stop_reason"`
	TimingLimited bool   `json:"timing_limited"`
}

// Budget is operation-local and must not be shared between concurrent searches.
// Box accounting conservatively includes inputs plus live work/output regions.
type Budget struct {
	ctx      context.Context
	limits   Limits
	stats    Stats
	deadline time.Time
	now      func() time.Time
	stopped  error
	closed   bool
	reserved int
}

func NewBudget(ctx context.Context, limits Limits) (*Budget, error) {
	return NewBudgetWithClock(ctx, limits, time.Now)
}

// NewBudgetWithClock injects the safety clock without global hooks. Replay and
// correctness tests can force a deadline independently of machine clock speed.
func NewBudgetWithClock(ctx context.Context, limits Limits, now func() time.Time) (*Budget, error) {
	d := Defaults()
	if ctx == nil || now == nil || limits.MaxOperations < 1 || limits.MaxOperations > d.MaxOperations || limits.MaxBoxes < 1 || limits.MaxBoxes > d.MaxBoxes || limits.Timeout <= 0 || limits.Timeout > d.Timeout {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Budget{ctx: ctx, limits: limits, deadline: now().Add(limits.Timeout), now: now}, nil
}

func (b *Budget) Close() {
	if b != nil {
		b.closed = true
	}
}
func (b *Budget) Stats() Stats {
	if b == nil {
		return Stats{}
	}
	return b.stats
}

func (b *Budget) check() error {
	if b == nil || b.closed {
		return ErrInvalid
	}
	if b.stopped != nil {
		return b.stopped
	}
	if err := b.ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return b.stop("context_deadline", true, ErrLimit)
		}
		return b.stop("canceled", false, err)
	}
	if !b.now().Before(b.deadline) {
		return b.stop("wall_clock", true, ErrLimit)
	}
	return nil
}

func (b *Budget) stop(reason string, timing bool, err error) error {
	b.stats.StopReason, b.stats.TimingLimited, b.stopped = reason, timing, err
	return err
}

func (b *Budget) Tick(n int) error {
	if err := b.check(); err != nil {
		return err
	}
	if n < 0 {
		return ErrInvalid
	}
	if n > b.limits.MaxOperations-b.stats.Operations {
		return b.stop("operations", false, ErrLimit)
	}
	b.stats.Operations += n
	return nil
}

func (b *Budget) CheckBoxes(n int) error {
	if err := b.check(); err != nil {
		return err
	}
	if n < 0 {
		return ErrInvalid
	}
	if n > b.limits.MaxBoxes-b.reserved {
		return b.stop("boxes", false, ErrLimit)
	}
	total := n + b.reserved // Both are nonnegative and bounded before this addition.
	if total > b.stats.PeakBoxes {
		b.stats.PeakBoxes = total
	}
	return nil
}

// SetReserved accounts for live regions retained by the caller outside the
// current algebra operands. Do not reserve operands already counted by algebra.
// The caller must reserve before allocating or retaining those regions.
func (b *Budget) SetReserved(n int) error {
	if err := b.check(); err != nil {
		return err
	}
	if n < 0 {
		return ErrInvalid
	}
	if n > b.limits.MaxBoxes {
		return b.stop("boxes", false, ErrLimit)
	}
	b.reserved = n
	return b.CheckBoxes(0)
}
