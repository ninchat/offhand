package relink

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// boundConn
type boundConn struct {
	conn       net.Conn
	acceptTime time.Time
	reader     *alignReader
}

// binding
type binding struct {
	binder *Binder
	name   string

	lock   sync.Mutex
	conns  chan *boundConn
	closed bool
}

func (bi *binding) init(b *Binder, name string) {
	bi.binder = b
	bi.name = name
	bi.conns = make(chan *boundConn)
}

func (bi *binding) close() {
	bi.binder.lock.Lock()
	if bi.binder.bindings != nil {
		delete(bi.binder.bindings, bi.name)
	}
	bi.binder.lock.Unlock()

	bi.lock.Lock()
	if !bi.closed {
		close(bi.conns)
		bi.closed = true
	}
	bi.lock.Unlock()
}

// Binder
type Binder struct {
	Listener net.Listener

	HandshakeTimeout time.Duration

	Logger Logger
	Stats  *Stats

	lock     sync.Mutex
	bindings map[string]*binding
	closed   bool
}

func (b *Binder) bind(bi *binding, name string) (err error) {
	nameData := []byte(name)
	if len(nameData) > 255 {
		err = fmt.Errorf("endpoint name %s raw length %d is over 255 bytes", name, len(nameData))
		return
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if b.bindings == nil {
		if b.Listener == nil {
			panic("Binder.Listener is missing")
		}

		if b.HandshakeTimeout == 0 {
			b.HandshakeTimeout = DefaultHandshakeTimeout
		}

		if b.Logger == nil {
			b.Logger = defaultLogger{}
		}

		if b.Stats == nil {
			b.Stats = new(Stats)
		}

		b.bindings = make(map[string]*binding)

		go b.acceptLoop()
	}

	if b.bindings[name] != nil {
		err = ErrBindingAlreadyExists
		return
	}

	bi.init(b, name)
	b.bindings[name] = bi
	return
}

// Close might return ErrBinderClosed.
func (b *Binder) Close() (err error) {
	b.lock.Lock()

	bindings := b.bindings
	b.bindings = nil

	closed := b.closed
	b.closed = true

	b.lock.Unlock()

	if closed {
		err = ErrBinderClosed
		return
	}

	if bindings != nil {
		b.Listener.Close()

		for _, binding := range bindings {
			binding.close()
		}
	}

	return
}

func (b *Binder) acceptLoop() {
	for {
		if c, err := b.Listener.Accept(); err != nil {
			if b.closed {
				break
			}

			b.Logger.Printf("listen %v: %v", b.Listener, err)
			time.Sleep(time.Second)
		} else {
			go b.accepted(c)
		}
	}
}

func (b *Binder) accepted(c net.Conn) {
	start := time.Now()

	name, r, err := b.earlyHandshake(c, start)
	if err != nil {
		c.Close()
		b.Logger.Printf("accepted %v: %v", connString(c), err)
		return
	}

	var binding *binding

	b.lock.Lock()
	if !b.closed {
		binding = b.bindings[name]
	}
	b.lock.Unlock()

	if binding == nil {
		c.Close()

		if !b.closed {
			b.Logger.Printf("accepted %v: unknown endpoint name %s from connector", connString(c), name)
		}

		return
	}

	binding.lock.Lock()
	if !binding.closed {
		binding.conns <- &boundConn{c, start, r}
	}
	binding.lock.Unlock()
}

func (b *Binder) earlyHandshake(c net.Conn, now time.Time) (name string, r *alignReader, err error) {
	var deadline time.Time

	if b.HandshakeTimeout >= 0 {
		deadline = now.Add(jitter(b.HandshakeTimeout, 0.25))
	}

	if err = c.SetDeadline(deadline); err != nil {
		return
	}

	if err = readHandshakeVersion(c); err != nil {
		return
	}

	if err = writeHandshakeVersion(c); err != nil {
		return
	}

	r = newAlignReader(c)

	nameData, err := readHandshakeEndpoint(r)
	if err != nil {
		return
	}

	name = string(nameData)
	return
}
