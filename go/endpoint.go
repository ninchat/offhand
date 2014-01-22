package offhand

import (
	"sync"
)

type Endpoint struct {
	outgoing  Options
	incoming  Options
	mutex     sync.Mutex
	closing   bool
	dialers   map[*Dialer]bool
	listeners map[*Listeners]bool
	sockets   map[*socket]bool
	online    map[*socket]bool
	sequence  uint64
	free      map[uint64]*socket
	busy      map[uint64]*socket
}

func NewEndpoint(outgoing Options, incoming Options) *Endpoint {
	return &Endpoint{
		outgoing:  outgoing,
		incoming:  incoming,
		dialers:   make(map[*Dialer]bool),
		listeners: make(map[*Listener]bool),
		sockets:   make(map[*socket]bool),
		online:    make(map[*socket]bool),
		free:      make(map[uint64]*socket),
		busy:      make(map[uint64]*socket),
	}
}

func (e *Endpoint) Close() (err error) {
	e.mutex.Lock()

	if e.closing {
		return
	}

	e.closing = true

	e.mutex.Unlock()

	for d, _ := range e.dialers {
		d.Close()
	}

	for l, _ := range e.listeners {
		l.Close()
	}

	for s, _ := range e.sockets {
		s.close()
	}

	return
}

func (e *Endpoint) Dial(network, addr string) (d *Dialer) {
	s := new(socket)

	d = &Dialer{
		endpoint: e,
		socket:   s,
	}

	e.mutex.Lock()

	if !e.closing {
		e.dialers[d] = true
		e.sockets[s] = true
	} else {
		d = nil
	}

	e.mutex.Unlock()

	if d != nil {
		go d.dial(network, addr)
	}

	return
}

func (e *Endpoint) Listen(network, addr string) (l *Listener) {
	l = &Listener{
		endpoint: e,
	}

	e.mutex.Lock()

	if !e.closing {
		e.listeners[l] = true
	} else {
		l = nil
	}

	e.mutex.Unlock()

	if l != nil {
		go l.listen(network, addr)
	}

	return
}

func (e *Endpoint) dialer_closed(s *socket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	delete(e.dialers, s)
}

func (e *Endpoint) listener_closed(s *socket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	delete(e.listeners, s)
}

func (e *Endpoint) socket_closed(s *socket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	delete(e.sockets, s)
}

func (e *Endpoint) socket_online(s *socket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.online[s] = true
}

func (e *Endpoint) socket_offline(s *socket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	delete(e.online, s)
}

func (e *Endpoint) new_listener_socket() (s *socket, id uint64) {
	s = new(socket)

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.sockets[s] = true

	e.sequence++
	id = e.sequence
	e.busy[id] = s
	return
}

func (e *Endpoint) lookup_listener_socket(id uint64) (s *socket) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	s = free[id]
	if s != nil {
		delete(free, s)
		busy[id] = s
		return
	}

	s = busy[id]
	if s != nil {
		panic("TODO: break off from old connection")
	}
	return
}
