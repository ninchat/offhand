package relink

import (
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ChannelId is a primitive type which is determined by ChannelTraits.
type ChannelId interface{}

// ChannelMap wraps a map indexed with a ChannelId implementation.
type ChannelMap interface {
	// Insert adds an id to the map.
	Insert(ChannelId, interface{})

	// Lookup finds an id in the map.
	Lookup(ChannelId) interface{}

	// Delete removes an id from the map.
	Delete(ChannelId)

	// Length gets the number of items in the map.
	Length() int

	// Foreach calls a function for each item.
	Foreach(func(ChannelId, interface{}))
}

// ChannelTraits provides operations for a concrete channel id type.
type ChannelTraits interface {
	// IdSize returns the size of ChannelId's underlying type in bytes.
	IdSize() int

	// CreateMap allocates a new ChannelMap.
	CreateMap() ChannelMap

	// CreateAndReadIds allocates one or more ChannelIds and fills them.
	CreateAndReadIds(ids []ChannelId, r io.Reader) error
}

// ChannelOptions specifies protocol options for single transport direction.
type ChannelOptions struct {
	Transactions bool          // sender commits messages
	Traits       ChannelTraits // specifies the channel id size
}

func (o *ChannelOptions) init() (err error) {
	if o.Traits != nil {
		idSize := o.Traits.IdSize()

		if idSize < 0 || idSize > 255 {
			return fmt.Errorf("channel id size %d not in range [0,255]", idSize)
		}

		if idSize == 0 {
			o.Traits = nil
		}
	}

	return
}

func (o *ChannelOptions) idSize() (size uint8) {
	if o.Traits != nil {
		size = uint8(o.Traits.IdSize())
	}

	return
}

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

		if c.Id == nil {
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
		if c.Id == nil {
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

func (s *outgoingSender) sendPackets(w *alignWriter, traits ChannelTraits, ids []ChannelId, sentPackets, sentMessages *int) (err error) {
	for _, om := range s.messages {
		if err = writeMessagePacket(w, traits, ids, om.Message); err != nil {
			return
		}
	}

	(*sentPackets) += len(s.messages)
	(*sentMessages) += len(s.messages)

	if s.close {
		if err = writeChannelOpPacket(w, traits, ids, protocolChannelOpClose); err != nil {
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

	// The rest are protected by Link.xmitLock, except messageConsumeSequence.

	queued bool

	messageReceived uint32
	messageReceiver chan *IncomingMessage

	messageConsumeSequence uint32
	messageConsumeNotify   chan bool
	messageRewindNotify    chan bool

	messageConsumed     uint32
	messageConsumedCond sync.Cond
	messageConsumeSent  uint32

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
		c.messageConsumeNotify = make(chan bool, 1)
		c.messageRewindNotify = make(chan bool, 1)
		c.messageConsumedCond.L = &c.Link.xmitLock

		go c.bufferLoop(bufferSize)
	}

	c.messageReceived++

	return &IncomingMessage{
		Message:  m,
		Sequence: MessageSequence(c.messageReceived),
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

		var rewind bool

		select {
		case im := <-inputChan:
			if im == nil {
				receiver = nil
				continue
			}

			unconsumed = append(unconsumed, im)
			continue

		case outputChan <- outputItem:
			delivered++
			continue

		case <-timeChan:
			t := c.Link.updateTransmission()

			if c.messageConsumed != c.messageConsumeSent && !c.queued && !c.Link.xmitPeerShutdown {
				t.enqueue(c)
				c.queued = true
			}

			t.done()

			timeChan = nil
			continue

		case <-c.messageConsumeNotify:

		case <-c.messageRewindNotify:
			rewind = true
		}

		// consume and (optionally) rewind

		sequence := atomic.LoadUint32(&c.messageConsumeSequence)
		consumeCount := sequence - c.messageConsumed

		if consumeCount > uint32(delivered) {
			min := c.messageConsumed
			max := min + uint32(delivered)

			if c.Id == nil {
				c.Link.Endpoint.Logger.Printf("%v: consumed sequence %d out of modular range [%d,%d]", c.Link, sequence, min, max)
			} else {
				c.Link.Endpoint.Logger.Printf("%v: consumed sequence %d out of modular range [%d,%d] on channel %v", c.Link, sequence, min, max, c.Id)
			}

			continue
		}

		var ack bool

		t := c.Link.updateTransmission()

		c.messageConsumed = sequence

		if int(c.messageConsumed-c.messageConsumeSent) >= c.Link.Endpoint.MessageAckWindow && !c.queued && !c.Link.xmitPeerShutdown {
			t.enqueue(c)
			c.queued = true
			ack = true
		}

		t.done()

		c.messageConsumedCond.Broadcast()

		if ack && timeChan != nil {
			timer.Stop()
			timeChan = nil
		}

		if !ack && timeChan == nil {
			timer = time.NewTimer(jitter(c.Link.Endpoint.MessageAckDelay, -0.1))
			timeChan = timer.C
		}

		unconsumed = unconsumed[consumeCount:]

		// release memory
		if len(unconsumed) == 0 {
			unconsumed = nil
		}

		if rewind {
			delivered = 0
		} else {
			delivered -= int(consumeCount)
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

// Consume
func (c *IncomingChannel) Consume(seq MessageSequence) {
	oldSeq := atomic.SwapUint32(&c.messageConsumeSequence, uint32(seq))
	if uint32(seq) == oldSeq {
		return
	}

	c.consumeWait(c.messageConsumeNotify, oldSeq)
}

// ConsumeAndRewind
func (c *IncomingChannel) ConsumeAndRewind(seq MessageSequence) {
	oldSeq := atomic.SwapUint32(&c.messageConsumeSequence, uint32(seq))
	if uint32(seq) == oldSeq {
		return
	}

	c.consumeWait(c.messageRewindNotify, oldSeq)
}

// Rewind
func (c *IncomingChannel) Rewind() {
	oldSeq := atomic.LoadUint32(&c.messageConsumeSequence)
	c.consumeWait(c.messageRewindNotify, oldSeq)
}

// consumeWait
func (c *IncomingChannel) consumeWait(notify chan<- bool, oldSeq uint32) {
	select {
	case notify <- true:
	default:
	}

	c.Link.xmitLock.Lock()
	defer c.Link.xmitLock.Unlock()

	// this doesn't work right if we get scheduled for the first time when the
	// sequence has advanced exactly 2^32
	for c.messageConsumed == oldSeq {
		c.messageConsumedCond.Wait()
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

func (s *incomingSender) sendPackets(w *alignWriter, traits ChannelTraits, ids []ChannelId, sentPackets *int) (err error) {
	if s.received {
		if err = writeSequenceAckPacket(w, traits, ids, protocolSequenceAckReceived, s.receivedSequence); err != nil {
			return
		}

		(*sentPackets)++
	}

	if s.consumed {
		if err = writeSequenceAckPacket(w, traits, ids, protocolSequenceAckConsumed, s.consumedSequence); err != nil {
			return
		}

		(*sentPackets)++
	}

	if s.closed {
		if err = writeChannelAckPacket(w, traits, ids, protocolChannelAckClosed); err != nil {
			return
		}

		(*sentPackets)++
	}

	return
}

// outgoingChannelMap adapts ChannelMap for *OutgoingChannel values.
type outgoingChannelMap struct {
	impl ChannelMap
}

func (m outgoingChannelMap) Insert(id ChannelId, c *OutgoingChannel) {
	m.impl.Insert(id, c)
}

func (m outgoingChannelMap) Lookup(id ChannelId) (c *OutgoingChannel) {
	if x := m.impl.Lookup(id); x != nil {
		c = x.(*OutgoingChannel)
	}

	return
}

func (m outgoingChannelMap) Delete(id ChannelId) {
	m.impl.Delete(id)
}

func (m outgoingChannelMap) Length() int {
	return m.impl.Length()
}

func (m outgoingChannelMap) Foreach(f func(*OutgoingChannel)) {
	m.impl.Foreach(func(id ChannelId, x interface{}) {
		f(x.(*OutgoingChannel))
	})
}

// incomingChannelMap adapts ChannelMap for *IncomingChannel values.
type incomingChannelMap struct {
	impl ChannelMap
}

func (m incomingChannelMap) Insert(id ChannelId, c *IncomingChannel) {
	m.impl.Insert(id, c)
}

func (m incomingChannelMap) Lookup(id ChannelId) (c *IncomingChannel) {
	if x := m.impl.Lookup(id); x != nil {
		c = x.(*IncomingChannel)
	}

	return
}

func (m incomingChannelMap) Delete(id ChannelId) {
	m.impl.Delete(id)
}

func (m incomingChannelMap) Length() int {
	return m.impl.Length()
}

func (m incomingChannelMap) Foreach(f func(*IncomingChannel)) {
	m.impl.Foreach(func(id ChannelId, x interface{}) {
		f(x.(*IncomingChannel))
	})
}

// singleChannelMap always finds the same value, until it loses it.
type singleChannelMap struct {
	value interface{}
}

func (m *singleChannelMap) Insert(id ChannelId, value interface{}) {
	panic("singleChannelMap.Insert called")
}

func (m *singleChannelMap) Lookup(id ChannelId) interface{} {
	if id != nil {
		panic("non-nil ChannelId conflicts with ChannelOptions")
	}

	return m.value
}

func (m *singleChannelMap) Delete(id ChannelId) {
	if id != nil {
		panic("singleChannelMap.Delete called with non-nil id")
	}

	m.value = nil
}

func (m *singleChannelMap) Length() (n int) {
	if m.value != nil {
		n = 1
	}

	return
}

func (m *singleChannelMap) Foreach(f func(ChannelId, interface{})) {
	if m.value != nil {
		f(nil, m.value)
	}
}
