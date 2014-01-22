package offhand

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"time"
)

type Listener struct {
	endpoint *Endpoint
	mutex    sync.Mutex
	closing  bool
	netl     net.Listener
}

func (l *Listener) Close() (err error) {
	if l.closing {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.closing = true
	l.netl.Close()
	l.endpoint.listener_closed(l)
	return
}

func (l *Listener) listen(network, addr string) {
	for !l.closing {
		if netl, err := net.Listen(network, addr); err == nil {
			ok := false

			l.mutex.Lock()
			if !l.closing {
				l.netl = netl
				ok = true
			}
			l.mutex.Unlock()

			if ok {
				for !l.closing {
					if c, err := netl.Accept(); err == nil {
						go l.handle(c)
					}
				}

				l.mutex.Lock()
				l.netl = nil
				l.mutex.Unlock()
			}

			netl.Close()
		}

		if l.closing {
			break
		}

		time.Sleep(reconnect_delay)
	}
}

func (l *Listener) handle(c net.Conn) {
	defer c.Close()

	c.SetDeadline(time.Now().Add(handshake_timeout))

	my_version := byte(ProtocolVersion)

	if _, err := c.Write([]byte{my_version}); err != nil {
		return
	}

	peer_version := make([]byte, 1)

	if _, err := c.Read(peer_version); err != nil {
		return
	}

	if peer_version[0] != my_version {
		return
	}

	my_outgoing := l.endpoint.Outgoing.marshal()
	my_incoming := l.endpoint.Incoming.marshal()

	if _, err := c.Write([]byte{my_outgoing, my_incoming}); err != nil {
		return
	}

	peer_options := make([]byte, 3)

	if _, err := c.Read(peer_options); err != nil {
		return
	}

	peer_outcoming := peer_options[0]
	peer_ingoing := peer_options[1]
	peer_socket_id_size := int(peer_options[2])

	if peer_outcoming != my_incoming || peer_ingoing != my_outgoing {
		return
	}

	var s *socket

	if peer_socket_id_size > 0 {
		b := make([]byte, peer_socket_id_size)

		if _, err := c.Read(b); err != nil {
			return
		}

		if peer_socket_id_size <= binary.MaxVarintLen64 {
			if socket_id, err := binary.ReadUvarint(bytes.NewReader(b)); err == nil {
				s = l.endpoint.lookup_listener_socket(socket_id)
			}
		}
	}

	var my_socket_size_id []byte

	if s == nil {
		var socket_id uint64

		s, socket_id = l.endpoint.new_listener_socket()

		b := make([]byte, 1 + binary.MaxVarintLen64)
		n := binary.PutUvarint(b[1:], socket_id)
		b[0] = byte(n)

		my_socket_size_id = b[:n+1]
	} else {
		my_socket_size_id = []byte{byte(0)}
	}

	if _, err := c.Write(my_socket_size_id); err != nil {
		return
	}

	s.transfer(c)
}
