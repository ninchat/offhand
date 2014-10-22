package relink

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// endpointWorker
type endpointWorker interface {
	closeBegin()
	closeFinish()
}

// Endpoint is used to create links to remote endpoints.  An Endpoint value may
// be initialized freely, but if the Connect, Listen or Bind method is called,
// the Close method should be called at the end.
type Endpoint struct {
	OutgoingOptions ChannelOptions
	IncomingOptions ChannelOptions

	Links chan *Link

	MaxMulticastCount   int   // multicast channel id limit
	MaxMessageLength    int   // message part count limit
	MaxMessagePartSize  int   // limit for a message part's size
	MaxMessageTotalSize int64 // limit for the sum of message part sizes

	MessageSendWindow        int           // number of outgoing messages per channel
	MessageReceiveBufferSize int           // number of incoming messages per channel
	MessageAckWindow         int           // number of messages to receive before batch acknowledgement
	MessageAckDelay          time.Duration // how long to wait until batch ack of received messages

	HandshakeTimeout     time.Duration // -1 disables
	PacketReceiveTimeout time.Duration // -1 disables
	PacketSendTimeout    time.Duration // -1 disables

	PingInterval time.Duration // -1 disables
	PongTimeout  time.Duration

	MaxReconnectDelay time.Duration

	// TODO: implement this
	LinkTimeout time.Duration // -1 disables

	Logger Logger
	Stats  *Stats

	lock    sync.Mutex
	inited  bool
	closing bool
	workers map[endpointWorker]bool
}

func (e *Endpoint) init() (err error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.inited {
		if e.closing {
			err = ErrEndpointClosed
		}

		return
	}

	if e.MaxMulticastCount == 0 {
		e.MaxMulticastCount = DefaultMaxMulticastCount
	}
	if e.MaxMulticastCount < 0 || e.MaxMulticastCount > math.MaxInt32 {
		err = fmt.Errorf("Endpoint.MaxMulticastCount %v not in range [0,%v]", e.MaxMulticastCount, 0xffffff)
		return
	}

	if e.MaxMessageLength == 0 {
		e.MaxMessageLength = DefaultMaxMessageLength
	}
	if e.MaxMessageLength < 0 || e.MaxMessageLength > math.MaxInt32 {
		err = fmt.Errorf("Endpoint.MaxMessageLength %v not in range [0,%v]", e.MaxMessageLength, 0x7fffffff)
		return
	}

	if e.MaxMessagePartSize == 0 {
		e.MaxMessagePartSize = DefaultMaxMessagePartSize
	}
	if e.MaxMessagePartSize < 0 {
		err = fmt.Errorf("Endpoint.MaxMessagePartSize %v is invalid", e.MaxMessagePartSize)
		return
	}

	if e.MaxMessageTotalSize == 0 {
		e.MaxMessageTotalSize = DefaultMaxMessageTotalSize
	}
	if e.MaxMessageTotalSize < 0 {
		err = fmt.Errorf("Endpoint.MaxMessageTotalSize %v is invalid", e.MaxMessageTotalSize)
		return
	}

	if e.MessageSendWindow == 0 {
		e.MessageSendWindow = DefaultMessageSendWindow
	}
	if e.MessageSendWindow < 1 || e.MessageSendWindow > math.MaxInt32 {
		err = fmt.Errorf("Endpoint.MessageSendWindow %v not in range [1,%v]", e.MessageSendWindow, math.MaxInt32)
		return
	}

	if e.MessageReceiveBufferSize == 0 {
		e.MessageReceiveBufferSize = DefaultMessageReceiveBufferSize
	}
	if e.MessageReceiveBufferSize < 1 || e.MessageReceiveBufferSize > math.MaxInt32 {
		err = fmt.Errorf("Endpoint.MessageReceiveBufferSize %v not in range [1,%v]", e.MessageReceiveBufferSize, math.MaxInt32)
		return
	}

	if e.MessageAckWindow == 0 {
		e.MessageAckWindow = DefaultMessageAckWindow
	}
	if e.MessageAckWindow < 1 || e.MessageAckWindow > math.MaxInt32 {
		err = fmt.Errorf("Endpoint.MessageAckWindow %v not in range [1,%v]", e.MessageAckWindow, math.MaxInt32)
		return
	}

	if e.MessageAckDelay == 0 {
		e.MessageAckDelay = DefaultMessageAckDelay
	}

	if e.HandshakeTimeout == 0 {
		e.HandshakeTimeout = DefaultHandshakeTimeout
	}

	if e.PacketSendTimeout == 0 {
		e.PacketSendTimeout = DefaultPacketSendTimeout
	}

	if e.PacketReceiveTimeout == 0 {
		e.PacketReceiveTimeout = DefaultPacketReceiveTimeout
	}

	if e.PingInterval == 0 {
		e.PingInterval = DefaultPingInterval
	}

	if e.PongTimeout == 0 {
		e.PongTimeout = DefaultPongTimeout
	}

	if e.MaxReconnectDelay == 0 {
		e.MaxReconnectDelay = DefaultMaxReconnectDelay
	}

	if e.LinkTimeout == 0 {
		e.LinkTimeout = DefaultLinkTimeout
	}

	if e.Logger == nil {
		e.Logger = defaultLogger{}
	}

	if e.Stats == nil {
		e.Stats = new(Stats)
	}

	e.workers = make(map[endpointWorker]bool)

	e.inited = true
	return
}

// Close stops creating new links and closes the existing ones.  The Links
// channel will be closed after all links have shut down.  May return
// ErrEndpointClosed among others.  This call blocks.
func (e *Endpoint) Close() (err error) {
	var workers map[endpointWorker]bool

	e.lock.Lock()
	if e.inited && !e.closing {
		workers = e.workers
		e.workers = make(map[endpointWorker]bool)
	}
	e.closing = true
	e.lock.Unlock()

	if workers == nil {
		err = ErrEndpointClosed
		return
	}

	for w := range workers {
		w.closeBegin()
	}

	for w := range workers {
		w.closeFinish()
	}

	if e.Links != nil {
		close(e.Links)
	}

	return
}

func (e *Endpoint) transfer(l *Link, connClosed chan<- bool) {
	if e.Links != nil {
		e.Links <- l
	}

	l.transferLoop(connClosed)
}

// endpointConnector
type endpointConnector struct {
	endpoint *Endpoint

	connector Connector
	link      *Link
	name      []byte
	flags     uint8
	epoch     int64

	// protected by endpoint.lock
	closed bool
}

// Connect opens a link by connecting to a peer.  May return ErrEndpointClosed
// among others.
func (e *Endpoint) Connect(c Connector, name string) (l *Link, err error) {
	if err = e.init(); err != nil {
		return
	}

	ec := &endpointConnector{
		endpoint:  e,
		connector: c,
		link:      newLink(e, 0, nil),
		name:      []byte(name),
	}

	if e.OutgoingOptions.Transactions {
		ec.flags |= protocolHandshakeFlagConnectorTransactions
	}

	if e.IncomingOptions.Transactions {
		ec.flags |= protocolHandshakeFlagListenerTransactions
	}

	if e.Links != nil {
		ec.flags |= protocolHandshakeFlagRequireOldLink
	}

	if len(ec.name) > 255 {
		err = fmt.Errorf("endpoint name %s raw length %d is over 255 bytes", name, len(ec.name))
		return
	}

	e.lock.Lock()
	ok := !e.closing
	if ok {
		e.workers[ec] = true
	}
	e.lock.Unlock()

	if !ok {
		err = ErrEndpointClosed
		return
	}

	go ec.connectLoop()

	l = ec.link
	return
}

func (ec *endpointConnector) closeBegin() {
	ec.link.Close()
}

func (ec *endpointConnector) closeFinish() {
	ec.link.waitClose()
	ec.closed = true
	ec.connector.Close()
}

func (ec *endpointConnector) connectLoop() {
	connClosed := make(chan bool)

	var backoff backoff

	for {
		var delay time.Duration

		c, err := ec.connector.Connect()

		if ec.closed {
			if c != nil {
				c.Close()
			}

			break
		}

		if err != nil {
			ec.endpoint.Logger.Printf("connect %v: %v", ec.connector, err)
			delay = backoff.Failure(ec.endpoint.MaxReconnectDelay)
		} else {
			ok, lost, err := ec.handshake(c, connClosed)

			if err != nil {
				ec.endpoint.Logger.Printf("connected %v: %v", connString(c), err)
			}

			if lost {
				break
			}

			if ok {
				if ok := <-connClosed; !ok {
					break
				}

				backoff.Success()
			} else {
				c.Close()
				delay = backoff.Failure(ec.endpoint.MaxReconnectDelay)
			}

			if ec.closed {
				break
			}
		}

		if delay > 0 {
			time.Sleep(delay)

			if ec.closed {
				break
			}
		}
	}

	ec.link.Close()

	ec.endpoint.lock.Lock()
	delete(ec.endpoint.workers, ec)
	ec.endpoint.lock.Unlock()

	// TODO: move this to a Link method
	if ec.link.id == 0 {
		close(ec.link.Errors)

		if ec.link.IncomingMessages != nil {
			close(ec.link.IncomingMessages)
		}

		if ec.link.IncomingChannels != nil {
			close(ec.link.IncomingChannels)
		}

		for _, channel := range ec.link.xmitIncoming {
			channel.close()
		}
	} else {
		for _ = range connClosed {
		}
	}
}

func (ec *endpointConnector) handshake(c net.Conn, connClosed chan<- bool) (ok, lost bool, err error) {
	if err = c.SetDeadline(deadline(ec.endpoint.HandshakeTimeout)); err != nil {
		return
	}

	if err = writeHandshakeVersion(c); err != nil {
		return
	}

	if err = readHandshakeVersion(c); err != nil {
		return
	}

	w := newAlignWriter(bufio.NewWriterSize(c, handshakeBufferSize))

	if err = writeHandshakeEndpoint(w, ec.name); err != nil {
		return
	}

	connFlags := ec.flags
	if ec.link.id == 0 {
		connFlags &^= protocolHandshakeFlagRequireOldLink
	}

	if err = writeHandshakeConfig(w, ec.endpoint.OutgoingOptions, ec.endpoint.IncomingOptions, connFlags); err != nil {
		return
	}

	if err = writeHandshakeIdent(w, ec.epoch, ec.link.id); err != nil {
		return
	}

	if err = w.Flush(); err != nil {
		return
	}

	ident, err := readHandshakeIdent(c)
	if err != nil {
		return
	}

	if ident.Epoch <= 0 {
		err = fmt.Errorf("bad epoch %d from peer", ident.Epoch)
		return
	}

	if (connFlags & protocolHandshakeFlagRequireOldLink) != 0 {
		if ec.link.id != 0 {
			if ident.LinkId == 0 {
				ec.link.lost(ErrLinkLost)
				lost = true
				return
			}

			if ident.LinkId != ec.link.id && ident.Epoch == ec.epoch {
				err = fmt.Errorf("new link id %d with old epoch %d from peer when old link id %d required", ident.LinkId, ec.epoch, ec.link.id)
				return
			}

			if ident.Epoch != ec.epoch {
				err = fmt.Errorf("new epoch %d and link id %d from peer when old epoch %d and link id %d required", ident.Epoch, ident.LinkId, ec.epoch, ec.link.id)
				return
			}
		}
	}

	if ident.LinkId == 0 {
		err = errors.New("no link id from peer")
		return
	}

	if ec.link.id == 0 {
		ec.link.init(ident.LinkId, c)
		ec.epoch = ident.Epoch
		ok = true

		go ec.endpoint.transfer(ec.link, connClosed)
	} else {
		if ident.Epoch != ec.epoch || ident.LinkId != ec.link.id {
			/* TODO */ err = errors.New("link replacement not implemented")
			return
		}

		if ok = ec.link.resetConn(c, nil); !ok {
			return
		}
	}

	return
}

// endpointListener
type endpointListener struct {
	endpoint *Endpoint

	binding    binding
	flags      uint8
	epoch      int64
	lastLinkId uint64

	// protected by endpoint.lock
	links       map[linkId]*Link
	linkRemoved sync.Cond
}

// Listen allows peers to connect to this endpoint.  May return
// ErrEndpointClosed among others.
func (e *Endpoint) Listen(l net.Listener, name string) (err error) {
	b := &Binder{
		Listener:         l,
		HandshakeTimeout: e.HandshakeTimeout,
		Logger:           e.Logger,
		Stats:            e.Stats,
	}

	return e.Bind(b, name)
}

// Bind allows peers to connect to this endpoint via a shared listening
// address.  May return ErrBinderClosed, ErrBindingAlreadyExists or
// ErrEndpointClosed among others.
func (e *Endpoint) Bind(b *Binder, name string) (err error) {
	if err = e.init(); err != nil {
		return
	}

	if e.Links == nil {
		err = fmt.Errorf("binding is pointless without Endpoint.Links")
		return
	}

	el := &endpointListener{
		endpoint: e,
		epoch:    time.Now().UnixNano() / 1000,
		links:    make(map[linkId]*Link),
	}

	el.linkRemoved.L = &el.endpoint.lock

	if e.OutgoingOptions.Transactions {
		el.flags |= protocolHandshakeFlagListenerTransactions
	}

	if e.IncomingOptions.Transactions {
		el.flags |= protocolHandshakeFlagConnectorTransactions
	}

	if err = b.bind(&el.binding, name); err != nil {
		return
	}

	e.lock.Lock()
	ok := !e.closing
	if ok {
		e.workers[el] = true
	}
	e.lock.Unlock()

	if ok {
		go el.acceptLoop()
	} else {
		err = ErrEndpointClosed
	}

	return
}

func (el *endpointListener) closeBegin() {
	for _, l := range el.links {
		l.Close()
	}
}

func (el *endpointListener) closeFinish() {
	for _, l := range el.links {
		l.waitClose()
	}

	el.endpoint.lock.Lock()
	for len(el.links) > 0 {
		el.linkRemoved.Wait()
	}
	el.endpoint.lock.Unlock()

	el.binding.close()
}

func (el *endpointListener) newLinkId() linkId {
	return linkId(atomic.AddUint64(&el.lastLinkId, 1))
}

func (el *endpointListener) insertLink(l *Link) (ok bool) {
	el.endpoint.lock.Lock()
	defer el.endpoint.lock.Unlock()

	if !el.endpoint.closing {
		el.links[l.id] = l
		ok = true
	}

	return
}

func (el *endpointListener) lookupLink(id linkId) *Link {
	el.endpoint.lock.Lock()
	defer el.endpoint.lock.Unlock()

	return el.links[id]
}

func (el *endpointListener) removeLink(id linkId) {
	el.endpoint.lock.Lock()
	defer el.endpoint.lock.Unlock()

	delete(el.links, id)
	el.linkRemoved.Broadcast()
}

func (el *endpointListener) acceptLoop() {
	for bc := range el.binding.conns {
		go el.accepted(bc)
	}
}

func (el *endpointListener) accepted(bc *boundConn) {
	ok, err := el.finishHandshake(bc.conn, bc.acceptTime, bc.reader)

	if err != nil {
		el.endpoint.Logger.Printf("accepted %v: %v", connString(bc.conn), err)
	}

	if !ok {
		bc.conn.Close()
	}
}

func (el *endpointListener) finishHandshake(c net.Conn, acceptTime time.Time, r *alignReader) (ok bool, err error) {
	var deadline time.Time

	if el.endpoint.HandshakeTimeout >= 0 {
		deadline = acceptTime.Add(jitter(el.endpoint.HandshakeTimeout, 0.25))
	}

	if err = c.SetDeadline(deadline); err != nil {
		return
	}

	config, err := readHandshakeConfig(r)
	if err != nil {
		return
	}

	if config.ConnectorChannelIdSize != byte(el.endpoint.IncomingOptions.IdSize) {
		err = fmt.Errorf("incompatible incoming channel id size %d from connector", config.ConnectorChannelIdSize)
		return
	}

	if config.ListenerChannelIdSize != byte(el.endpoint.OutgoingOptions.IdSize) {
		err = fmt.Errorf("incompatible outgoing channel id size %d from connector", config.ListenerChannelIdSize)
		return
	}

	txFlags := config.Flags & protocolHandshakeTransactionFlagMask
	if txFlags != (el.flags & protocolHandshakeTransactionFlagMask) {
		err = fmt.Errorf("incompatible transaction flags 0x%x from connector", txFlags)
		return
	}

	ident, err := readHandshakeIdent(r)
	if err != nil {
		return
	}

	if ident.LinkId != 0 {
		if ident.Epoch == 0 {
			err = fmt.Errorf("old link id %d without epoch from connector", ident.LinkId)
			return
		}

		var l *Link

		if ident.Epoch == el.epoch {
			l = el.lookupLink(ident.LinkId)
		}

		if l != nil {
			if err = writeHandshakeIdent(c, el.epoch, ident.LinkId); err != nil {
				return
			}

			ok = l.resetConn(c, nil)
			return
		} else if (config.Flags & protocolHandshakeFlagRequireOldLink) != 0 {
			err = writeHandshakeIdent(c, el.epoch, linkId(0))
			return
		}
	}

	id := el.newLinkId()
	l := newLink(el.endpoint, id, c)

	if ok = el.insertLink(l); !ok {
		return
	}

	if err = writeHandshakeIdent(c, el.epoch, id); err != nil {
		el.removeLink(id)
		return
	}

	go el.transfer(l)

	return
}

func (el *endpointListener) transfer(l *Link) {
	el.endpoint.transfer(l, nil)
	el.removeLink(l.id)
}
