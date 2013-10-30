package offhand

import (
	"encoding/binary"
	"net"
	"time"
)

type Commit struct {
	Message   [][]byte
	StartTime time.Time
	reply     chan byte
}

type Puller interface {
	Connect(addr string)
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

func (p *puller) Connect(addr string) {
	go p.loop(addr)
}

func (p *puller) RecvChannel() <-chan *Commit {
	return p.recv
}

func (p *puller) loop(addr string) {
	for {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}

		for {
			bytecode := make([]byte, 1)
			if _, err := conn.Read(bytecode); err != nil || bytecode[0] != begin_command {
				break
			}

			var message_size uint32
			if err := binary.Read(conn, binary.LittleEndian, &message_size); err != nil {
				break
			}

			payload := make([]byte, message_size)
			if _, err := conn.Read(payload); err != nil {
				break
			}

			bytecode[0] = received_reply
			if _, err := conn.Write(bytecode); err != nil {
				break
			}

			if _, err := conn.Read(bytecode); err != nil {
				break
			}
			if bytecode[0] == rollback_command {
				continue
			}
			if bytecode[0] != commit_command {
				break
			}

			var latency uint32
			if binary.Read(conn, binary.LittleEndian, &latency) != nil {
				break
			}

			start_time := time.Now().Add(time.Duration(latency) * -1000)

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

			commit_channel := make(chan byte)

			p.recv<- &Commit{
				Message:   message,
				StartTime: start_time,
				reply:     commit_channel,
			}

			bytecode[0] = <-commit_channel
			if _, err := conn.Write(bytecode); err != nil {
				break
			}
		}

		conn.Close()
	}
}

func (c *Commit) Engage() {
	c.reply<- engaged_reply
	c.reply = nil
}

func (c *Commit) Cancel() {
	c.reply<- canceled_reply
	c.reply = nil
}

func (c *Commit) Close() {
	if c.reply != nil {
		c.Cancel()
	}
}
