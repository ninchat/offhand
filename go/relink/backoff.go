package relink

import (
	"time"
)

const (
	maxBackoffSlots = 1024
)

// backoff
type backoff struct {
	lastSlot int
}

func (b *backoff) Success() {
	b.lastSlot = 0
}

func (b *backoff) Failure(maxDelay time.Duration) (delay time.Duration) {
	if b.lastSlot > 0 {
		delay = jitter(time.Duration(int64(maxDelay)*int64(b.lastSlot)/maxBackoffSlots), -0.5)
	}

	if b.lastSlot < maxBackoffSlots-1 {
		b.lastSlot = ((b.lastSlot + 1) << 1) - 1
	}

	return
}
