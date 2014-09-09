package relink

import (
	"math"
	"time"
)

// Message has zero or more binary parts.
type Message [][]byte

// MessageSequence is opaque data which shouldn't be accessed directly.
type MessageSequence uint32

// Add
func (seq MessageSequence) Add(offset int) MessageSequence {
	if offset < 0 || offset > math.MaxUint32 {
		panic("bad message sequence offset")
	}

	return MessageSequence(uint32(seq) + uint32(offset))
}

// Sub
func (seq MessageSequence) Sub(offset int) MessageSequence {
	if offset < 0 || offset > math.MaxUint32 {
		panic("bad message sequence offset")
	}

	return MessageSequence(uint32(seq) - uint32(offset))
}

// outgoingMessage
type outgoingMessage struct {
	Message

	ack chan error
}

// IncomingMessage
type IncomingMessage struct {
	Message
	Sequence MessageSequence

	channel *IncomingChannel
}

// Consume
func (im *IncomingMessage) Consume() Message {
	im.channel.Consume(im.Sequence)
	return im.Message
}

// Sender
type Sender interface {
	Send(payload Message, waitAck bool, deadline time.Time) (sent bool, err error)
	Close() error
}
