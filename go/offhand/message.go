package offhand

import (
	"math"
	"time"
)

// Message has zero or more binary parts.
type Message [][]byte

// IncomingMessageSequence is opaque data which shouldn't be accessed directly.
type IncomingMessageSequence uint32

// Add
func (seq IncomingMessageSequence) Add(offset int) IncomingMessageSequence {
	if offset < 0 || offset > math.MaxUint32 {
		panic("bad message sequence offset")
	}

	return IncomingMessageSequence(uint32(seq) + uint32(offset))
}

// Sub
func (seq IncomingMessageSequence) Sub(offset int) IncomingMessageSequence {
	if offset < 0 || offset > math.MaxUint32 {
		panic("bad message sequence offset")
	}

	return IncomingMessageSequence(uint32(seq) - uint32(offset))
}

// outgoingMessage
type outgoingMessage struct {
	Message

	ack chan error
}

// IncomingMessage
type IncomingMessage struct {
	Message
	Sequence IncomingMessageSequence
}

// Sender
type Sender interface {
	Send(payload Message, waitAck bool, deadline time.Time) (sent bool, err error)
	Close() error
}
