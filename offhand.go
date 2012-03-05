package offhand

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
)

const (
	begin_timeout    = time.Duration(10e9)
	commit_timeout   = time.Duration(60e9)
	tick_interval    = time.Duration( 3e9)

	begin_command    = byte(1)
	commit_command   = byte(2)
	rollback_command = byte(3)

	begin_reply      = byte(1)
	commit_reply     = byte(2)
)

type Pusher interface {
	SendMultipart(message [][]byte) error
	Close()
}

type pusher struct {
	listener   net.Listener
	ticker     *time.Ticker
	closing    bool
	closed     bool

	lock       sync.Mutex
	generation uint32
	payload    [][]byte
	begin      *sync.Cond
	committing bool
	committed  *sync.Cond
}

func NewListenPusher(l net.Listener) Pusher {
	p := &pusher{
		listener: l,
		ticker:   time.NewTicker(tick_interval),
	}

	p.begin     = sync.NewCond(&p.lock)
	p.committed = sync.NewCond(&p.lock)

	go p.accept_loop()
	go p.io_tick()

	return p
}

func (p *pusher) SendMultipart(message [][]byte) error {
	var message_size uint64
	var message_data = make([][]byte, 1 + len(message) * 2)

	for i, frame_data := range message {
		var frame_size = len(frame_data)
		var frame_head = make([]byte, 4)
		binary.LittleEndian.PutUint32(frame_head, uint32(frame_size))

		message_data[1 + i * 2 + 0] = frame_head
		message_data[1 + i * 2 + 1] = frame_data

		message_size += uint64(len(frame_head) + frame_size)
	}

	if message_size > 0xffffffff {
		return errors.New("message too long")
	}

	var message_head = make([]byte, 4)
	binary.LittleEndian.PutUint32(message_head, uint32(message_size))

	message_data[0] = message_head

	return p.send(message_data)
}

func (p *pusher) send(payload [][]byte) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	for {
		if p.closing {
			err = errors.New("pusher closed")
			return
		}

		if p.payload == nil {
			break
		}

		p.committed.Wait()
	}

	p.generation++
	p.payload    = payload
	p.committing = false

	p.begin.Signal()

	return
}

func (p *pusher) Close() {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.closing = true

	for p.payload != nil {
		p.committed.Wait()
	}

	p.closed = true

	p.begin.Broadcast()
	p.ticker.Stop()
	p.listener.Close()
}

func (p *pusher) accept_loop() {
	for {
		conn, err := p.listener.Accept()

		if p.closed {
			return
		}

		if err == nil {
			go p.io_loop(conn)
		}
	}
}

func (p *pusher) io_loop(conn net.Conn) {
	defer p.begin.Signal()
	defer conn.Close()

	for {
		var generation uint32
		var payload    [][]byte

		/* critical */ func() {
			p.lock.Lock()
			defer p.lock.Unlock()

			for {
				if p.payload != nil {
					if !p.committing {
						break
					}
				} else {
					if p.closing {
						return
					}
				}

				p.begin.Wait()
			}

			generation = p.generation
			payload    = p.payload
		}()

		if payload == nil {
			return
		}

		conn.SetDeadline(time.Now().Add(begin_timeout))

		if _, err := conn.Write([]byte{ begin_command }); err != nil {
			return
		}

		for _, buf := range payload {
			if _, err := conn.Write(buf); err != nil {
				return
			}
		}

		var buf = make([]byte, 1)

		if _, err := conn.Read(buf); err != nil || buf[0] != begin_reply {
			return
		}

		var command   = rollback_command
		var commanded = true
		var committed = false

		/* critical */ func() {
			p.lock.Lock()
			defer p.lock.Unlock()

			if p.generation == generation && p.payload != nil && !p.committing {
				p.committing = true
				command = commit_command
			}
		}()

		conn.SetDeadline(time.Now().Add(commit_timeout))

		if _, err := conn.Write([]byte{ command }); err != nil {
			commanded = false
		}

		if command == commit_command {
			if commanded {
				if _, err := conn.Read(buf); err == nil && buf[0] == commit_reply {
					committed = true
				}
			}

			/* critical */ func() {
				p.lock.Lock()
				defer p.lock.Unlock()

				p.committing = false

				if committed {
					p.payload = nil
					p.committed.Broadcast()
				}
			}()
		}

		if !committed {
			return
		}
	}
}

func (p *pusher) io_tick() {
	for _ = range p.ticker.C {
		p.begin.Signal()
	}
}
