package offhand

import (
	"crypto/tls"
	"encoding/binary"
	"net"
	"time"
)

type Commit struct {
	Message   [][]byte
	StartTime time.Time

	reply chan byte
}

type Puller interface {
	Connect(network, addr string, tlsConfig *tls.Config)
	RecvChannel() <-chan *Commit
}

type puller struct {
	recv chan *Commit
}

func NewPuller() Puller {
	return &puller{
		recv: make(chan *Commit),
	}
}

func (p *puller) Connect(network, addr string, tlsConfig *tls.Config) {
	go p.loop(network, addr, tlsConfig)
}

func (p *puller) RecvChannel() <-chan *Commit {
	return p.recv
}

func (p *puller) loop(network, addr string, tlsConfig *tls.Config) {
	for {
		conn, err := net.Dial(network, addr)
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}

	loop:
		for {
			bytecode := make([]byte, 1)
			if _, err := conn.Read(bytecode); err != nil {
				break
			}

			switch bytecode[0] {
			case starttlsCommand:
				if tlsConfig == nil {
					break loop
				}
				conn = tls.Client(conn, tlsConfig)
				tlsConfig = nil
				continue

			case keepaliveCommand:
				continue

			case beginCommand:
				// ok

			default:
				break loop
			}

			var messageSize uint32
			if err := binary.Read(conn, binary.LittleEndian, &messageSize); err != nil {
				break
			}

			payload := make([]byte, messageSize)
			if _, err := conn.Read(payload); err != nil {
				break
			}

			bytecode[0] = receivedReply
			if _, err := conn.Write(bytecode); err != nil {
				break
			}

			if _, err := conn.Read(bytecode); err != nil {
				break
			}
			if bytecode[0] == rollbackCommand {
				continue
			}
			if bytecode[0] != commitCommand {
				break
			}

			var latency uint32
			if binary.Read(conn, binary.LittleEndian, &latency) != nil {
				break
			}

			startTime := time.Now().Add(time.Duration(latency) * -1000)

			message := make([][]byte, 0)

			for len(payload) >= 4 {
				size := binary.LittleEndian.Uint32(payload[:4])
				payload = payload[4:]

				if len(payload) < int(size) {
					break
				}

				message = append(message, payload[:size])
				payload = payload[size:]
			}

			commitChannel := make(chan byte)

			p.recv <- &Commit{
				Message:   message,
				StartTime: startTime,
				reply:     commitChannel,
			}

			bytecode[0] = <-commitChannel
			if _, err := conn.Write(bytecode); err != nil {
				break
			}
		}

		conn.Close()
	}
}

func (c *Commit) Engage() {
	c.reply <- engagedReply
	c.reply = nil
}

func (c *Commit) Cancel() {
	c.reply <- canceledReply
	c.reply = nil
}

func (c *Commit) Close() {
	if c.reply != nil {
		c.Cancel()
	}
}
