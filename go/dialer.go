package offhand

import (
	"net"
	"time"
)

type Dialer struct {
	endpoint *Endpoint
	closing  bool
	id       []byte
}

func (d *Dialer) Close() (err error) {
	if d.closing {
		return
	}

	d.closing = true
	d.socket.close()
	d.endpoint.dialer_closed(d)
	return
}

func (d *Dialer) dial(network, addr string) {
	for !d.closing {
		if c, err := net.DialTimeout(network, addr, dial_timeout); err == nil {
			d.handle(c)
		}

		if d.closing {
			break
		}

		time.Sleep(reconnect_delay)
	}
}

func (d *Dialer) handle(c net.Conn) {
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

	my_outgoing := d.socket.endpoint.Outgoing.marshal()
	my_incoming := d.socket.endpoint.Incoming.marshal()

	my_header := make([]byte, 3 + len(d.id))
	my_header[0] = my_outgoing
	my_header[1] = my_incoming
	my_header[2] = byte(len(d.id))
	copy(my_header[3:], d.id)

	if _, err := c.Write(my_header); err != nil {
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

	if peer_socket_id_size > 0 {
		if len(d.id) > 0 {
			// TODO: reset old logical connection
		}

		d.id = make([]byte, peer_socket_id_size)

		if _, err := c.Read(d.id); err != nil {
			return
		}
	} else if len(d.id) == 0 {
		return
	}

	d.socket.transfer(c)
}
