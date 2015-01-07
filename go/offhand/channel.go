package offhand

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

// ChannelOptions specifies protocol options for single transport direction.
type ChannelOptions struct {
	Transactions bool // sender commits messages
	IdSize       int  // specifies the channel id array length
}

// ChannelId represents a fixed-length sequence of bytes.  The length is
// determined by ChannelOptions.  The only supported operations on this type
// are ChannelId([]byte) and []byte(ChannelId).
type ChannelId string

const noChannelId = ChannelId("")

// OutgoingChannel
type OutgoingChannel struct {
	Link *Link
	Id   ChannelId

	wait chan bool

	// The rest are protected by Link.xmitLock.

	queued   bool
	resuming bool

	messageConsumed  uint32
	messagesBuffered []*outgoingMessage
	messagesSent     int
	messagesReceived int

	closed    bool
	closeSent bool
}

func newOutgoingChannel(l *Link, id ChannelId) *OutgoingChannel {
	return &OutgoingChannel{
		Link:            l,
		Id:              id,
		wait:            make(chan bool, 1),
		messageConsumed: math.MaxUint32,
	}
}

func newClosedOutgoingChannel(l *Link, id ChannelId) *OutgoingChannel {
	return &OutgoingChannel{
		Link:            l,
		Id:              id,
		messageConsumed: math.MaxUint32,
		closed:          true,
		closeSent:       true,
	}
}

// Send sends a message to the peer.  See Link.SendMessage for more details;
// ErrChannelClosed may be returned also.
func (c *OutgoingChannel) Send(payload Message, waitAck bool, deadline time.Time) (sent bool, err error) {
	if c.closed {
		err = ErrChannelClosed
		return
	}

	return c.Link.send(c.Id, payload, waitAck, deadline)
}

// Close the channel.  May return ErrChannelClosed.
func (c *OutgoingChannel) Close() (err error) {
	if c.closed {
		err = ErrChannelClosed
		return
	}

	c.Link.closeChannel(c)
	return
}

// signal
func (c *OutgoingChannel) signal() {
	select {
	case c.wait <- true:
	default:
	}
}

// enqueue
func (c *OutgoingChannel) enqueue() (ok bool) {
	if !c.queued && c.messagesSent != len(c.messagesBuffered) || (c.closed && !c.closeSent) {
		c.queued = true
		ok = true
	}

	return
}

// beginResume is called with Link.xmitLock.
func (c *OutgoingChannel) beginResume() {
	c.resuming = true
}

// endResume is called with Link.xmitLock.
func (c *OutgoingChannel) endResume() (closed, enqueue bool, err error) {
	if !c.resuming {
		return
	}

	if c.closeSent {
		closed = true
		return
	}

	if c.messageConsumed != math.MaxUint32 {
		err = fmt.Errorf("channel %v with consumed messages was not resumed", c.Id)
		return
	}

	if c.messagesReceived > 0 {
		err = fmt.Errorf("channel %v with received messages was not resumed", c.Id)
		return
	}

	c.resuming = false
	c.messagesSent = 0
	enqueue = c.enqueue()
	return
}

// putMessage is called with Link.xmitLock.
func (c *OutgoingChannel) putMessage(om *outgoingMessage, sendWindow int) (ok, enqueue bool) {
	if c.closed {
		return
	}

	if len(c.messagesBuffered) >= sendWindow {
		return
	}

	c.messagesBuffered = append(c.messagesBuffered, om)
	ok = true
	enqueue = c.enqueue()
	return
}

// close is called with Link.xmitLock.
func (c *OutgoingChannel) close() (enqueue bool) {
	if c.closed {
		return
	}

	c.closed = true
	enqueue = c.enqueue()
	c.signal()
	return
}

// lost is called with Link.xmitLock.
func (c *OutgoingChannel) lost(err error) {
	for _, om := range c.messagesBuffered {
		if om.ack != nil {
			om.ack <- err
		}
	}
}

// onReceived is called with Link.xmitLock.
func (c *OutgoingChannel) onReceived(sequence uint32) (enqueue bool, err error) {
	received := int(sequence - c.messageConsumed)

	if received < c.messagesReceived || received > c.messagesSent {
		min := c.messageConsumed + uint32(c.messagesReceived)
		max := c.messageConsumed + uint32(c.messagesSent)

		if c.Id == noChannelId {
			err = fmt.Errorf("sequence %d received ack out of modular range [%d,%d]", sequence, min, max)
		} else {
			err = fmt.Errorf("sequence %d received ack out of modular range [%d,%d] on channel %v", sequence, min, max, c.Id)
		}
		return
	}

	// release memory
	for _, om := range c.messagesBuffered[c.messagesReceived:received] {
		om.Message = nil
	}

	c.messagesReceived = received

	if c.resuming {
		c.resuming = false
		c.messagesSent = received
		c.closeSent = false
	}

	enqueue = c.enqueue()
	return
}

// onReceivedNext is called with Link.xmitLock.
func (c *OutgoingChannel) onReceivedNext() (enqueue bool, err error) {
	return c.onReceived(c.messageConsumed + uint32(c.messagesReceived) + 1)
}

// onConsumed is called with Link.xmitLock.
func (c *OutgoingChannel) onConsumed(sequence uint32) (enqueue bool, err error) {
	lastAcked := c.messageConsumed
	lastSent := lastAcked + uint32(c.messagesSent)

	logicalLastAcked := int64(lastAcked)
	logicalLastSent := int64(lastSent)
	logicalAck := int64(sequence)

	if lastSent < lastAcked {
		logicalLastSent += 0x100000000

		if sequence <= lastSent {
			logicalAck += 0x100000000
		}
	}

	if logicalAck < logicalLastAcked || logicalAck > logicalLastSent {
		if c.Id == noChannelId {
			err = fmt.Errorf("sequence %d consumed ack out of modular range [%d,%d]", sequence, lastAcked, lastSent)
		} else {
			err = fmt.Errorf("sequence %d consumed ack out of modular range [%d,%d] on channel %v", sequence, lastAcked, lastSent, c.Id)
		}
		return
	}

	consume := int(logicalAck - logicalLastAcked)

	for _, om := range c.messagesBuffered[:consume] {
		if om.ack != nil {
			close(om.ack)
		}
	}

	c.messageConsumed += uint32(consume)
	c.messagesBuffered = c.messagesBuffered[consume:]
	c.messagesSent -= consume
	c.messagesReceived -= consume

	if c.messagesReceived <= 0 {
		c.messagesReceived = 0

		if c.resuming {
			c.resuming = false
			c.messagesSent = 0
			c.closeSent = false
		}
	}

	// release memory
	if len(c.messagesBuffered) == 0 {
		c.messagesBuffered = nil
	}

	enqueue = c.enqueue()
	c.signal()
	return
}

// onConsumedNext is called with Link.xmitLock.
func (c *OutgoingChannel) onConsumedNext() (enqueue bool, err error) {
	return c.onConsumed(c.messageConsumed + 1)
}

// isDrained is called with Link.xmitLock.
func (c *OutgoingChannel) isDrained() bool {
	return len(c.messagesBuffered) == 0
}

// getSender is called with Link.xmitLock.
func (c *OutgoingChannel) getSender() (ok bool, sender outgoingSender) {
	if !c.resuming {
		if len(c.messagesBuffered) > c.messagesSent {
			sender.messages = c.messagesBuffered[c.messagesSent:]
			c.messagesSent = len(c.messagesBuffered)
			ok = true
		}

		if c.closed && c.messagesSent == len(c.messagesBuffered) && !c.closeSent {
			sender.close = true
			c.closeSent = true
			ok = true
		}

		c.signal()
	}

	c.queued = false
	return
}

// outgoingSender sends an OutgoingChannel's queued packets.
type outgoingSender struct {
	messages []*outgoingMessage
	close    bool
}

func (s *outgoingSender) sendPackets(w *alignWriter, channelIdSize int, ids []ChannelId, sentPackets, sentMessages *int) (err error) {
	for _, om := range s.messages {
		if err = writeMessagePacket(w, channelIdSize, ids, om.Message); err != nil {
			return
		}
	}

	(*sentPackets) += len(s.messages)
	(*sentMessages) += len(s.messages)

	if s.close {
		if err = writeChannelOpPacket(w, channelIdSize, ids, protocolChannelOpClose); err != nil {
			return
		}

		(*sentPackets)++
	}

	return
}

// IncomingChannel keeps track of incoming message state.
type IncomingChannel struct {
	Link             *Link
	Id               ChannelId
	IncomingMessages chan *IncomingMessage

	// The rest are protected by Link.xmitLock, except messageConsumeLatest.

	queued bool

	messageReceived uint32
	messageReceiver chan *IncomingMessage

	messageConsumeLatest chan bool

	messageConsumeUntilSeq uint32
	messageConsumeUntil    chan bool

	messageConsumed    uint32
	messageConsumeSent uint32

	closed      bool
	closeToSend bool
}

func newIncomingChannel(l *Link, id ChannelId, messages chan *IncomingMessage) *IncomingChannel {
	return &IncomingChannel{
		Link:               l,
		Id:                 id,
		IncomingMessages:   messages,
		messageReceived:    math.MaxUint32,
		messageConsumed:    math.MaxUint32,
		messageConsumeSent: math.MaxUint32,
	}
}

// receiveEarly is called with Link.xmitLock.
func (c *IncomingChannel) receiveEarly(m Message, bufferSize int) *IncomingMessage {
	if c.messageReceiver == nil {
		c.messageReceiver = make(chan *IncomingMessage)
		c.messageConsumeLatest = make(chan bool, 1)
		c.messageConsumeUntil = make(chan bool, 1)

		go c.bufferLoop(bufferSize)
	}

	c.messageReceived++

	return &IncomingMessage{
		Message:  m,
		Sequence: IncomingMessageSequence(c.messageReceived),
		channel:  c,
	}
}

// receiveFinish
func (c *IncomingChannel) receiveFinish(im *IncomingMessage) {
	c.messageReceiver <- im
}

// close is called with Link.xmitLock.
func (c *IncomingChannel) close() {
	if c.closed {
		return
	}

	c.closed = true

	if c.messageReceiver != nil {
		close(c.messageReceiver)
	} else {
		close(c.IncomingMessages)
	}
}

// bufferLoop
func (c *IncomingChannel) bufferLoop(bufferSize int) {
	receiver := c.messageReceiver

	var unconsumed []*IncomingMessage
	var delivered int
	var timer *time.Timer
	var timeChan <-chan time.Time

	for receiver != nil || len(unconsumed) > 0 {
		var inputChan <-chan *IncomingMessage
		var outputChan chan<- *IncomingMessage
		var outputItem *IncomingMessage

		if len(unconsumed) < bufferSize {
			inputChan = receiver
		}

		if len(unconsumed) > delivered {
			outputChan = c.IncomingMessages
			outputItem = unconsumed[delivered]
		}

		select {
		case im := <-inputChan:
			if im == nil {
				receiver = nil
				break
			}

			unconsumed = append(unconsumed, im)

		case outputChan <- outputItem:
			delivered++

		case <-timeChan:
			t := c.Link.updateTransmission()

			if c.messageConsumed != c.messageConsumeSent && !c.queued && !c.Link.xmitPeerShutdown {
				t.enqueue(c)
				c.queued = true
			}

			t.done()

			timeChan = nil

		case <-c.messageConsumeLatest:
			if delivered == 0 {
				if c.Id == noChannelId {
					c.Link.Endpoint.Logger.Printf("%v: consuming latest but nothing delivered", c.Link)
				} else {
					c.Link.Endpoint.Logger.Printf("%v: consuming latest but nothing delivered on channel %v", c.Link, c.Id)
				}

				break
			}

			if delivered == 1 {
				var ack bool

				t := c.Link.updateTransmission()

				c.messageConsumed++

				if int(c.messageConsumed-c.messageConsumeSent) >= c.Link.Endpoint.MessageAckWindow && !c.queued && !c.Link.xmitPeerShutdown {
					t.enqueue(c)
					c.queued = true
					ack = true
				}

				t.done()

				if ack && timeChan != nil {
					timer.Stop()
					timeChan = nil
				}

				if !ack && timeChan == nil {
					timer = time.NewTimer(jitter(c.Link.Endpoint.MessageAckDelay, -0.1))
					timeChan = timer.C
				}
			}

			if len(consumed) == 1 {
				unconsumed = nil
				delivered = 0
			} else {
				delivered--
				unconsumed = unconsumed[:]
			}

		case <-c.messageConsumeUntil:
			consumeUntil := atomic.LoadUint32(&c.messageConsumeUntilSeq)
			untilCount := consumeUntil - c.messageConsumed

			if untilCount > 0 {
				if untilCount > uint32(delivered) {
					min := c.messageConsumed
					max := min + uint32(delivered)

					if c.Id == noChannelId {
						c.Link.Endpoint.Logger.Printf("%v: consumed until-sequence %d out of modular range [%d,%d]", c.Link, consumeUntil, min, max)
					} else {
						c.Link.Endpoint.Logger.Printf("%v: consumed until-sequence %d out of modular range [%d,%d] on channel %v", c.Link, consumeUntil, min, max, c.Id)
					}

					break
				}

				var ack bool

				t := c.Link.updateTransmission()

				c.messageConsumed = consumeUntil

				if int(c.messageConsumed-c.messageConsumeSent) >= c.Link.Endpoint.MessageAckWindow && !c.queued && !c.Link.xmitPeerShutdown {
					t.enqueue(c)
					c.queued = true
					ack = true
				}

				t.done()

				if ack && timeChan != nil {
					timer.Stop()
					timeChan = nil
				}

				if !ack && timeChan == nil {
					timer = time.NewTimer(jitter(c.Link.Endpoint.MessageAckDelay, -0.1))
					timeChan = timer.C
				}

				unconsumed = unconsumed[untilCount:]
				delivered -= int(untilCount)

				// release memory
				if len(unconsumed) == 0 {
					unconsumed = nil
				}
			}
		}
	}

	if timeChan != nil {
		timer.Stop()
	}

	close(c.IncomingMessages)

	t := c.Link.updateTransmission()

	if !c.Link.xmitPeerShutdown {
		c.closeToSend = true

		if !c.queued {
			t.enqueue(c)
			c.queued = true
		}
	}

	t.done()
}

// ConsumeLatest
func (c *IncomingChannel) ConsumeLatest() {
	select {
	case c.messageConsumeLatest <- true:
	default:
	}
}

// ConsumeUntil
func (c *IncomingChannel) ConsumeUntil(seq IncomingMessageSequence) {
	atomic.StoreUint32(&c.messageConsumeUntilSeq, uint32(seq))

	select {
	case c.messageConsumeUntil <- true:
	default:
	}
}

// getSender is called with Link.xmitLock.
func (c *IncomingChannel) getSender(resuming bool) (ok bool, sender incomingSender, closed bool) {
	if c.messageReceiver == nil {
		return
	}

	if resuming /* && c.messageReceived != c.messageConsumed */ {
		sender.received = true
		sender.receivedSequence = c.messageReceived
		ok = true
	}

	if c.messageConsumed != c.messageConsumeSent || resuming {
		c.messageConsumeSent = c.messageConsumed
		sender.consumed = true
		sender.consumedSequence = c.messageConsumeSent
		ok = true
	}

	if c.closeToSend {
		sender.closed = true
		closed = true
		ok = true
	}

	c.queued = false
	return
}

// incomingSender sends an IncomingChannel's queued packets.
type incomingSender struct {
	received         bool
	receivedSequence uint32

	consumed         bool
	consumedSequence uint32

	closed bool
}

func (s *incomingSender) sendPackets(w *alignWriter, channelIdSize int, ids []ChannelId, sentPackets *int) (err error) {
	if s.received {
		if err = writeSequenceAckPacket(w, channelIdSize, ids, protocolSequenceAckReceived, s.receivedSequence); err != nil {
			return
		}

		(*sentPackets)++
	}

	if s.consumed {
		if err = writeSequenceAckPacket(w, channelIdSize, ids, protocolSequenceAckConsumed, s.consumedSequence); err != nil {
			return
		}

		(*sentPackets)++
	}

	if s.closed {
		if err = writeChannelAckPacket(w, channelIdSize, ids, protocolChannelAckClosed); err != nil {
			return
		}

		(*sentPackets)++
	}

	return
}
