package offhand

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	packetDataReceiveBufferSize = 4096
	packetDataSendBufferSize    = 4096
)

// linkId
type linkId uint64

// Link between a local and a remote endpoint.
type Link struct {
	Endpoint *Endpoint

	// IncomingMessages is used to receive messages when the channel id size is
	// zero.  It is closed after the peer has shut down.
	IncomingMessages chan *IncomingMessage

	// IncomingChannels notifies about open channels when the channel id size
	// is non-zero.  It is closed after the peer has shut down.
	IncomingChannels chan *IncomingChannel

	// Errors may deliver either ErrLinkLost or ErrLinkTimeout.  It is closed
	// after the Close method has been called successfully or an error has been
	// delivered.
	Errors chan error

	id linkId

	xmitLock         sync.Mutex
	xmitQueue        []interface{}
	xmitNotify       chan bool
	xmitClosed       bool
	xmitSelfResuming int
	xmitPeerResuming bool
	xmitSelfShutdown bool
	xmitPeerShutdown bool
	xmitOutgoing     map[ChannelId]*OutgoingChannel
	xmitIncoming     map[ChannelId]*IncomingChannel

	connLock  sync.Mutex
	conn      net.Conn
	connLost  error
	connReset sync.Cond

	pingTime int64
}

type linkTransmissionUpdate struct {
	link   *Link
	notify bool
}

// newLink.  If c is nil, init must be called before transfer.
func newLink(e *Endpoint, id linkId, c net.Conn) (l *Link) {
	l = &Link{
		Endpoint:         e,
		Errors:           make(chan error, 1),
		id:               id,
		xmitNotify:       make(chan bool, 1),
		xmitSelfResuming: -1,
		xmitOutgoing:     make(map[ChannelId]*OutgoingChannel),
		xmitIncoming:     make(map[ChannelId]*IncomingChannel),
		conn:             c,
	}

	if e.OutgoingOptions.IdSize == 0 {
		l.xmitOutgoing[noChannelId] = newOutgoingChannel(l, noChannelId)
	}

	if e.IncomingOptions.IdSize > 0 {
		l.IncomingChannels = make(chan *IncomingChannel)
	} else {
		l.IncomingMessages = make(chan *IncomingMessage)
		l.xmitIncoming[noChannelId] = newIncomingChannel(l, noChannelId, l.IncomingMessages)
	}

	l.connReset.L = &l.connLock

	return
}

// init must be called if and only if conn is nil.
func (l *Link) init(id linkId, c net.Conn) {
	l.connLock.Lock()
	defer l.connLock.Unlock()

	l.id = id
	l.conn = c
}

func (l *Link) updateTransmission() (t linkTransmissionUpdate) {
	t.link = l
	t.link.xmitLock.Lock()
	return
}

func (t *linkTransmissionUpdate) enqueue(channel interface{}) {
	t.link.xmitQueue = append(t.link.xmitQueue, channel)
	t.notify = true
}

func (t *linkTransmissionUpdate) done() {
	t.link.xmitLock.Unlock()

	if t.notify {
		select {
		case t.link.xmitNotify <- true:
		default:
		}
	}
}

// String describes current connection state, if any.
func (l *Link) String() string {
	return l.string(l.conn)
}

func (l *Link) string(c net.Conn) string {
	if c != nil {
		return fmt.Sprintf("link %d conn %v", l.id, connString(c))
	} else {
		return fmt.Sprintf("link %d", l.id)
	}
}

// Close initiates link shutdown.  ErrLinkClosed is returned if the method has
// already been called.  ErrLinkLost or ErrLinkTimeout is returned if the peer
// has died.
func (l *Link) Close() (err error) {
	t := l.updateTransmission()

	if l.xmitClosed {
		l.connLock.Lock()

		if l.connLost != nil {
			err = l.connLost
		} else {
			err = ErrLinkClosed
		}

		l.connLock.Unlock()
	} else {
		l.xmitClosed = true
		t.notify = true

		if l.Endpoint.OutgoingOptions.IdSize > 0 {
			for _, channel := range l.xmitOutgoing {
				if enqueue := channel.close(); enqueue {
					t.enqueue(channel)
				}
			}
		} else {
			if channel := l.xmitOutgoing[noChannelId]; channel.isDrained() {
				delete(l.xmitOutgoing, noChannelId)
			}
		}
	}

	t.done()

	return
}

// waitClose blocks until link has shut down.
func (l *Link) waitClose() {
	l.connLock.Lock()

	for l.id == 0 || l.conn != nil {
		l.connReset.Wait()
	}

	l.connLock.Unlock()
}

// Send sends a message to the peer when the channel id size is zero.
// ErrLinkClosed is returned if the Close method was called before or during
// sending.  ErrMessageTimeout is returned if deadline is non-zero and the send
// buffer was full, or if waitAck is true and the peer didn't acknowledge the
// message in time.  ErrLinkLost or ErrLinkTimeout is returned if waitAck is
// true and the peer died before it could acknowledge the message.  sent is set
// to true if the message was buffered for sending; note that it may be true
// even when an error is returned, if waitAck is true.
//
// TODO: move the waitAck functionality to a separate WaitConsumed method?
//       Send should return a ticket which is passed to WaitConsumed?
func (l *Link) Send(payload Message, waitAck bool, deadline time.Time) (sent bool, err error) {
	return l.send(noChannelId, payload, waitAck, deadline)
}

func (l *Link) send(id ChannelId, payload Message, waitAck bool, deadline time.Time) (sent bool, err error) {
	om := &outgoingMessage{
		Message: payload,
	}

	if waitAck {
		om.ack = make(chan error, 1)
	}

	var timer *time.Timer
	var timeChan <-chan time.Time

	initTimer := func() (err error) {
		if timer == nil && !deadline.IsZero() {
			d := deadline.Sub(time.Now())
			if d <= 0 {
				err = ErrMessageTimeout
				return
			}

			timer = time.NewTimer(d)
			timeChan = timer.C
		}

		return
	}

	var prevChannel *OutgoingChannel

	for {
		var channel *OutgoingChannel
		var wait bool

		t := l.updateTransmission()

		if l.xmitClosed {
			err = ErrLinkClosed
		} else {
			channel = l.xmitOutgoing[id]
			if channel == nil {
				channel = newOutgoingChannel(l, id)
				l.xmitOutgoing[id] = channel
				t.enqueue(channel)
			}

			var enqueue bool

			if sent, enqueue = channel.putMessage(om, l.Endpoint.MessageSendWindow); sent {
				if enqueue {
					t.enqueue(channel)
				}
			} else {
				wait = true
			}
		}

		t.done()

		if prevChannel != nil && channel != prevChannel {
			prevChannel.signal()
		}

		if !wait {
			if channel != nil && (sent || l.xmitClosed) {
				channel.signal()
			}

			break
		}

		if err = initTimer(); err != nil {
			return
		}

		select {
		case <-channel.wait:
			// next iteration

		case <-timeChan:
			err = ErrMessageTimeout
			return
		}

		prevChannel = channel
	}

	if waitAck {
		select {
		case err = <-om.ack:

		case <-timeChan:
			err = ErrMessageTimeout
		}
	}

	if timer != nil {
		timer.Stop()
	}

	return
}

// Consume
func (l *Link) Consume(seq MessageSequence) {
	if channel := l.xmitIncoming[noChannelId]; channel != nil {
		channel.Consume(seq)
	}
}

// ConsumeAndRewind
func (l *Link) ConsumeAndRewind(seq MessageSequence) {
	if channel := l.xmitIncoming[noChannelId]; channel != nil {
		channel.ConsumeAndRewind(seq)
	}
}

// OutgoingChannel returns a handle for sending messages on a channel.
func (l *Link) OutgoingChannel(id ChannelId) (c *OutgoingChannel) {
	t := l.updateTransmission()

	c = l.xmitOutgoing[id]
	if c == nil {
		if l.xmitClosed {
			c = newClosedOutgoingChannel(l, id)
		} else {
			c = newOutgoingChannel(l, id)
			l.xmitOutgoing[id] = c
		}
	}

	t.done()

	return
}

// IncomingChannel returns a handle for receiving messages on a channel.
func (l *Link) IncomingChannel(id ChannelId) (c *IncomingChannel) {
	t := l.updateTransmission()

	c = l.xmitIncoming[id]
	if c == nil {
		c = newIncomingChannel(l, id, make(chan *IncomingMessage))
		l.xmitIncoming[id] = c
	}

	t.done()

	if l.xmitPeerShutdown {
		c.close()
	}

	return
}

func (l *Link) closeChannel(c *OutgoingChannel) {
	t := l.updateTransmission()
	if enqueue := c.close(); enqueue {
		t.enqueue(c)
	}
	t.done()
}

func (l *Link) lost(err error) {
	l.resetConn(nil, err)

	t := l.updateTransmission()

	l.xmitClosed = true

	for _, channel := range l.xmitOutgoing {
		channel.lost(err)
	}

	for _, channel := range l.xmitIncoming {
		channel.close()
	}

	t.done()
}

func (l *Link) resetConn(c net.Conn, lost error) (ok bool) {
	l.connLock.Lock()

	if l.conn != nil {
		l.conn.Close()
		l.conn = c
		l.connLost = lost
		l.connReset.Broadcast()
		ok = true
	}

	l.connLock.Unlock()
	return
}

func (l *Link) transferLoop(connClosed chan<- bool) {
	var prev net.Conn
	var lost error

	for {
		var curr net.Conn

		l.connLock.Lock()

		for {
			curr = l.conn
			lost = l.connLost

			if curr != prev {
				break
			}

			l.connReset.Wait()
		}

		l.connLock.Unlock()

		if curr == nil {
			break
		}

		if prev != nil {
			l.resume()
		}

		l.transferConn(curr)

		if l.xmitSelfShutdown && l.xmitPeerShutdown {
			break
		}

		if connClosed != nil {
			connClosed <- true
		}

		prev = curr
	}

	if connClosed != nil {
		go close(connClosed)
	}

	l.resetConn(nil, nil)

	if !l.xmitSelfShutdown {
		if lost != nil {
			l.Errors <- lost
		}

		close(l.Errors)
	}

	if !l.xmitPeerShutdown {
		if l.IncomingChannels != nil {
			close(l.IncomingChannels)
		}

		for _, channel := range l.xmitIncoming {
			channel.close()
		}
	}
}

func (l *Link) resume() {
	t := l.updateTransmission()

	for _, channel := range l.xmitOutgoing {
		channel.beginResume()
	}

	for _, channel := range l.xmitIncoming {
		if !channel.queued {
			t.enqueue(channel)
		}
	}

	l.xmitSelfResuming = len(l.xmitIncoming)
	l.xmitPeerResuming = true

	t.done()
}

func (l *Link) transferConn(c net.Conn) {
	generalLoop := make(chan uint8, 1)

	go l.sendLoop(c, generalLoop)

	if err := l.receiveLoop(c, generalLoop); err != nil {
		l.logConnError(c, err)
	}

	close(generalLoop)
}

func (l *Link) logConnError(c net.Conn, err error) {
	if l.conn != nil {
		l.Endpoint.Logger.Printf("%s: %v", l.string(c), err)
	}
}

func (l *Link) sendLoop(c net.Conn, generalLoop <-chan uint8) {
	w := newAlignWriter(bufio.NewWriterSize(c, packetDataSendBufferSize))

	var err error

	if l.xmitSelfShutdown {
		err = l.sendGeneralPacket(c, w, protocolGeneralShutdown)

		if err == nil {
			err = w.Flush()
		}
	}

	if err == nil {
		for {
			var ok bool

			if ok, err = l.sendPackets(c, w, generalLoop); !ok {
				break
			}
		}
	}

	if err == nil {
		err = w.Flush()
	}

	if err != nil {
		l.logConnError(c, err)
	}

	for _ = range generalLoop {
	}
}

func (l *Link) sendPackets(c net.Conn, w *alignWriter, generalLoop <-chan uint8) (ok bool, err error) {
	var sentPackets int
	var sentMessages int
	var sentShutdown bool

	for {
		select {
		case generalPacketType, open := <-generalLoop:
			if !open {
				return
			}

			if err = l.sendGeneralPacket(c, w, generalPacketType); err != nil {
				return
			}

			sentPackets++

		default:
			// don't block
		}

		var id ChannelId
		var sendOutgoing bool
		var sendIncoming bool
		var outgoingSender outgoingSender
		var incomingSender incomingSender

		l.xmitLock.Lock()

		if len(l.xmitQueue) > 0 {
			switch channel := l.xmitQueue[0].(type) {
			case *OutgoingChannel:
				id = channel.Id
				sendOutgoing, outgoingSender = channel.getSender()

			case *IncomingChannel:
				var resuming bool
				var closed bool

				if l.xmitSelfResuming > 0 {
					resuming = true
					l.xmitSelfResuming--
				}

				id = channel.Id
				sendIncoming, incomingSender, closed = channel.getSender(resuming)

				if closed {
					delete(l.xmitIncoming, id)
				}
			}

			l.xmitQueue = l.xmitQueue[1:]

			// release memory
			if len(l.xmitQueue) == 0 {
				l.xmitQueue = nil
			}
		}

		sendShutdown := l.xmitClosed && (len(l.xmitOutgoing) == 0) && !l.xmitSelfShutdown

		l.xmitLock.Unlock()

		if err = c.SetWriteDeadline(deadline(l.Endpoint.PacketSendTimeout)); err != nil {
			return
		}

		if sendOutgoing {
			if err = outgoingSender.sendPackets(w, l.Endpoint.OutgoingOptions.IdSize, []ChannelId{id}, &sentPackets, &sentMessages); err != nil {
				return
			}
		}

		if sendIncoming {
			if err = incomingSender.sendPackets(w, l.Endpoint.IncomingOptions.IdSize, []ChannelId{id}, &sentPackets); err != nil {
				return
			}
		}

		if l.xmitSelfResuming == 0 {
			if err = l.sendGeneralPacket(c, w, protocolGeneralResume); err != nil {
				return
			}

			sentPackets++
			l.xmitSelfResuming = -1
		}

		if sendShutdown {
			if err = l.sendGeneralPacket(c, w, protocolGeneralShutdown); err != nil {
				return
			}

			sentPackets++
			sentShutdown = true
		}

		if !(sendOutgoing || sendIncoming) {
			break
		}
	}

	if err = w.Flush(); err != nil {
		return
	}

	atomic.AddUint64(&l.Endpoint.Stats.SentPackets, uint64(sentPackets))
	atomic.AddUint64(&l.Endpoint.Stats.SentMessages, uint64(sentMessages))

	if sentShutdown {
		close(l.Errors)

		l.xmitLock.Lock()
		l.xmitSelfShutdown = true
		terminate := l.xmitPeerShutdown
		l.xmitLock.Unlock()

		if terminate {
			l.resetConn(nil, nil)
			return
		}
	}

	l.xmitLock.Lock()
	terminate := l.xmitSelfShutdown && l.xmitPeerShutdown
	l.xmitLock.Unlock()

	if terminate {
		return
	}

	select {
	case generalPacketType, open := <-generalLoop:
		if !open {
			return
		}

		if err = l.sendGeneralPacket(c, w, generalPacketType); err != nil {
			return
		}

		if err = w.Flush(); err != nil {
			return
		}

		atomic.AddUint64(&l.Endpoint.Stats.SentPackets, 1)

	case _ = <-l.xmitNotify:
		// next iteration
	}

	ok = true
	return
}

func (l *Link) sendGeneralPacket(c net.Conn, w *alignWriter, packetType uint8) (err error) {
	if err = c.SetWriteDeadline(deadline(l.Endpoint.PacketSendTimeout)); err != nil {
		return
	}

	if err = writeGeneralPacket(w, packetType); err != nil {
		return
	}

	if packetType == protocolGeneralPing {
		atomic.StoreInt64(&l.pingTime, time.Now().UnixNano())
	}

	return
}

func (l *Link) receiveLoop(c net.Conn, generalLoop chan<- uint8) (err error) {
	r := newAlignReader(bufio.NewReaderSize(c, packetDataReceiveBufferSize))

	for {
		l.xmitLock.Lock()
		terminate := l.xmitSelfShutdown && l.xmitPeerShutdown
		l.xmitLock.Unlock()

		if terminate {
			return
		}

		if err = c.SetReadDeadline(deadline(l.Endpoint.PingInterval)); err != nil {
			return
		}

		var channel bool
		var multicast bool
		var format uint8
		var x uint8

		if channel, multicast, format, x, err = readPacketHeader(r); err != nil {
			if operr, ok := err.(*net.OpError); ok && operr.Timeout() {
				if t := atomic.LoadInt64(&l.pingTime); t == 0 {
					select {
					case generalLoop <- protocolGeneralPing:
					default:
					}
				} else if (time.Now().UnixNano() - t) > jitter(l.Endpoint.PongTimeout, 0.1).Nanoseconds() {
					err = fmt.Errorf("pong timeout %v exceeded", l.Endpoint.PongTimeout)
					return
				}

				continue
			}

			return
		}

		if err = c.SetReadDeadline(deadline(l.Endpoint.PacketReceiveTimeout)); err != nil {
			return
		}

		var shortMessageLength uint8

		if shortMessageLength, err = readPacketShortMessageLength(r); err != nil {
			return
		}

		if channel {
			if l.xmitPeerShutdown {
				switch format {
				case protocolFormatChannelOp, protocolFormatSequenceOp, protocolFormatMessage:
					err = fmt.Errorf("channel packet format %d after shutdown", format)
					return
				}
			}

			var channelIdSize int

			if (format & protocolFormatFlagReceiveChannel) != 0 {
				channelIdSize = l.Endpoint.OutgoingOptions.IdSize
			} else {
				channelIdSize = l.Endpoint.IncomingOptions.IdSize
			}

			var ids []ChannelId

			if multicast {
				ids, err = readMulticastPacketChannelIds(r, channelIdSize, l.Endpoint.MaxMulticastCount)
			} else {
				ids, err = readUnicastPacketChannelIds(r, channelIdSize)
			}

			if err != nil {
				return
			}

			switch format {
			case protocolFormatChannelOp:
				err = l.receiveChannelOpPacket(r, ids, x)

			case protocolFormatChannelAck:
				err = l.receiveChannelAckPacket(r, ids, x)

			case protocolFormatSequenceOp:
				err = l.receiveSequenceOpPacket(r, ids, x)

			case protocolFormatSequenceAck:
				err = l.receiveSequenceAckPacket(r, ids, x)

			case protocolFormatMessage:
				err = l.receiveMessagePacket(r, ids, x, shortMessageLength)

			default:
				err = fmt.Errorf("unknown channel packet format %d", format)
			}
		} else {
			err = l.receiveGeneralPacket(c, x, generalLoop)
		}

		if err != nil {
			return
		}

		if err = r.Align(8); err != nil {
			return
		}

		atomic.AddUint64(&l.Endpoint.Stats.ReceivedPackets, 1)
	}
}

func (l *Link) receiveGeneralPacket(c net.Conn, packetType uint8, generalLoop chan<- uint8) (err error) {
	switch packetType {
	case protocolGeneralNop:
		// no operation

	case protocolGeneralPing:
		generalLoop <- protocolGeneralPong

	case protocolGeneralPong:
		atomic.StoreInt64(&l.pingTime, 0)

	case protocolGeneralResume:
		t := l.updateTransmission()

		var remove []ChannelId

		for _, channel := range l.xmitOutgoing {
			closed, enqueue, e := channel.endResume()
			if e != nil {
				if err != nil {
					l.Endpoint.Logger.Printf("%s: %v", l.string(c), err)
				}
				err = e
			} else if closed {
				remove = append(remove, channel.Id)
			} else if enqueue {
				t.enqueue(channel)
			}
		}

		for _, id := range remove {
			delete(l.xmitOutgoing, id)
		}

		l.xmitPeerResuming = false

		t.done()

	case protocolGeneralShutdown:
		var terminate bool

		t := l.updateTransmission()

		if !l.xmitPeerShutdown {
			l.xmitPeerShutdown = true
			terminate = l.xmitSelfShutdown

			if l.IncomingChannels != nil {
				close(l.IncomingChannels)
			}

			for _, channel := range l.xmitIncoming {
				channel.close()
			}
		}

		t.done()

		if terminate {
			l.resetConn(nil, nil)
		}

	default:
		err = fmt.Errorf("unknown general packet type %d", packetType)
	}

	return
}

func (l *Link) receiveChannelOpPacket(r io.Reader, ids []ChannelId, op uint8) (err error) {
	for _, id := range ids {
		switch op {
		case protocolChannelOpCommit:
			/* TODO */ err = errors.New("channel commit-op not implemented")

		case protocolChannelOpRollback:
			/* TODO */ err = errors.New("channel rollback-op not implemented")

		case protocolChannelOpClose:
			t := l.updateTransmission()
			if channel := l.xmitIncoming[id]; channel != nil {
				channel.close()
			}
			t.done()

		default:
			err = fmt.Errorf("unknown channel op packet type %d", op)
		}

		if err != nil {
			return
		}
	}

	return
}

func (l *Link) receiveChannelAckPacket(r io.Reader, ids []ChannelId, ack uint8) (err error) {
	for _, id := range ids {
		t := l.updateTransmission()

		if channel := l.xmitOutgoing[id]; channel != nil {
			var enqueue bool

			switch ack {
			case protocolChannelAckReceived:
				enqueue, err = channel.onReceivedNext()

			case protocolChannelAckConsumed:
				enqueue, err = channel.onConsumedNext()

				if l.xmitClosed && l.Endpoint.OutgoingOptions.IdSize == 0 && channel.isDrained() {
					delete(l.xmitOutgoing, id)
				}

			case protocolChannelAckCommitted:
				/* TODO */ err = errors.New("channel commited-ack not implemented")

			case protocolChannelAckUncommitted:
				/* TODO */ err = errors.New("channel uncommited-ack not implemented")

			case protocolChannelAckClosed:
				delete(l.xmitOutgoing, id)
				channel.signal()

				if l.xmitClosed && len(l.xmitOutgoing) == 0 {
					t.notify = true
				}

			default:
				err = fmt.Errorf("unknown channel ack packet type %d", ack)
			}

			if err == nil && enqueue {
				t.enqueue(channel)
			}
		} else if l.xmitPeerResuming && id != noChannelId {
			err = fmt.Errorf("peer claims to know nonexistent channel %v after reconnect", id)
		}

		t.done()

		if err != nil {
			return
		}
	}

	return
}

func (l *Link) receiveSequenceOpPacket(r *alignReader, ids []ChannelId, op uint8) (err error) {
	if _, err = readPacketSequence(r); err != nil {
		return
	}

	for _ = range ids {
		switch op {
		case protocolSequenceOpCommit:
			/* TODO */ err = errors.New("sequence commit-op not implemented")

		case protocolSequenceOpRollback:
			/* TODO */ err = errors.New("sequence rollback-op not implemented")

		default:
			err = fmt.Errorf("unknown sequence op packet type %d", op)
		}

		if err != nil {
			return
		}
	}

	return
}

func (l *Link) receiveSequenceAckPacket(r *alignReader, ids []ChannelId, ack uint8) (err error) {
	sequence, err := readPacketSequence(r)
	if err != nil {
		return
	}

	for _, id := range ids {
		t := l.updateTransmission()

		if channel := l.xmitOutgoing[id]; channel != nil {
			var enqueue bool

			switch ack {
			case protocolSequenceAckReceived:
				enqueue, err = channel.onReceived(sequence)

			case protocolSequenceAckConsumed:
				enqueue, err = channel.onConsumed(sequence)

				if l.xmitClosed && l.Endpoint.OutgoingOptions.IdSize == 0 && channel.isDrained() {
					delete(l.xmitOutgoing, id)
				}

			case protocolSequenceAckCommitted:
				/* TODO */ err = errors.New("sequence committed-ack not implemented")

			case protocolSequenceAckUncommitted:
				/* TODO */ err = errors.New("sequence uncommitted-ack not implemented")

			default:
				err = fmt.Errorf("unknown sequence ack packet type %d", ack)
			}

			if err == nil && enqueue {
				t.enqueue(channel)
			}
		} else if l.xmitPeerResuming && id != noChannelId {
			err = fmt.Errorf("peer claims to know nonexistent channel %v after reconnect", id)
		}

		t.done()

		if err != nil {
			return
		}
	}

	return
}

func (l *Link) receiveMessagePacket(r *alignReader, ids []ChannelId, flags, shortLength uint8) (err error) {
	if err = r.Align(2); err != nil {
		return
	}

	var length uint32

	if (flags & protocolMessageFlagLong) != 0 {
		if length, err = readLongMessagePacketLength(r); err != nil {
			return
		}
	} else {
		length = uint32(shortLength)
	}

	var payload Message

	if length > 0 {
		if length > uint32(l.Endpoint.MaxMessageLength) {
			err = fmt.Errorf("message length %d exceeds limit %d", length, l.Endpoint.MaxMessageLength)
			return
		}

		payload = make([][]byte, int(length))

		if (flags & protocolMessageFlagLarge) != 0 {
			err = readLargeMessagePacketPayload(r, payload, l.Endpoint.MaxMessagePartSize, l.Endpoint.MaxMessageTotalSize)
		} else {
			err = readSmallMessagePacketPayload(r, payload, l.Endpoint.MaxMessagePartSize, l.Endpoint.MaxMessageTotalSize)
		}
	} else {
		payload = make(Message, 0)
	}

	atomic.AddUint64(&l.Endpoint.Stats.ReceivedMessages, 1)

	for _, id := range ids {
		var channelCreated bool

		t := l.updateTransmission()

		channel := l.xmitIncoming[id]
		if channel == nil {
			channel = newIncomingChannel(l, id, make(chan *IncomingMessage))
			l.xmitIncoming[id] = channel
			channelCreated = true
		}

		im := channel.receiveEarly(payload, l.Endpoint.MessageReceiveBufferSize)

		t.done()

		if channelCreated && l.IncomingChannels != nil {
			l.IncomingChannels <- channel
		}

		channel.receiveFinish(im)
	}

	return
}
