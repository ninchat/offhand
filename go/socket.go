package offhand

import (
	"net"
	"time"
)

type socket struct {
	endpoint *Endpoint
	outgoing chan *packet
}

func (s *socket) close() {
	close(s.outgoing)
	s.endpoint.socket_closed(s)
}

func (s *socket) transfer(c net.Conn) {
	go s.send(c)
	s.receive(c)
}

func (s *socket) receive(c net.Conn) {
	for {
		c.SetReadDeadline(time.Now().Add(packet_timeout))

		// TODO
	}
}

func (s *socket) send(c net.Conn) {
	for packet := range s.outgoing {
		c.SetWriteDeadline(time.Now().Add(transfer_timeout))

		// TODO
	}
}
