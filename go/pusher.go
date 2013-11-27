package offhand

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Stats struct {
	Queue    uint32
	Send     uint32
	Error    uint32
	Rollback uint32
	Cancel   uint32
}

type Pusher interface {
	SendMultipart(message [][]byte, message_time time.Time) error
	Close()
	Stats() *Stats
}

type pusher struct {
	listener   net.Listener
	logger     func(error)
	ticker     *time.Ticker
	closing    bool
	closed     bool

	lock       sync.Mutex
	generation uint32
	payload    [][]byte
	start_time time.Time
	begin      *sync.Cond
	committing bool
	committed  *sync.Cond

	stats      Stats
}

func NewListenPusher(l net.Listener, logger func(error)) Pusher {
	p := &pusher{
		listener: l,
		logger:   logger,
		ticker:   time.NewTicker(tick_interval),
	}

	p.begin     = sync.NewCond(&p.lock)
	p.committed = sync.NewCond(&p.lock)

	go p.accept_loop()
	go p.io_tick()

	return p
}

func (p *pusher) SendMultipart(message [][]byte, message_time time.Time) error {
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

	return p.send(message_data, message_time)
}

func (p *pusher) send(payload [][]byte, start_time time.Time) (err error) {
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
	p.start_time = start_time
	p.committing = false

	p.begin.Signal()

	atomic.AddUint32(&p.stats.Queue, 1)

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
			p.initial_error(err)
			return
		}

		for _, buf := range payload {
			if _, err := conn.Write(buf); err != nil {
				p.initial_error(err)
				return
			}
		}

		var buf = make([]byte, 1)

		_, err := conn.Read(buf)
		if err == nil && buf[0] != received_reply {
			err = errors.New("bad reply to begin command")
		}
		if err != nil {
			p.initial_error(err)
			return
		}

		commit := false

		/* critical */ func() {
			p.lock.Lock()
			defer p.lock.Unlock()

			if p.generation == generation && p.payload != nil && !p.committing {
				p.committing = true
				commit = true
			}
		}()

		conn.SetDeadline(time.Now().Add(commit_timeout))

		commanded := false

		if commit {
			if _, err := conn.Write([]byte{ commit_command }); err != nil {
				p.log(err)
			} else {
				latency := uint32(time.Now().Sub(p.start_time).Nanoseconds() / 1000)
				if binary.Write(conn, binary.LittleEndian, &latency) == nil {
					commanded = true
				}
			}
		} else {
			if _, err := conn.Write([]byte{ rollback_command }); err != nil {
				p.log(err)
			} else {
				commanded = true
			}
		}

		reply := no_reply

		if commit {
			var err error

			if commanded {
				_, err = conn.Read(buf)
				if err == nil {
					reply = buf[0]
				}
			}

			/* critical */ func() {
				p.lock.Lock()
				defer p.lock.Unlock()

				p.committing = false

				switch reply {
				case engaged_reply:
					p.payload = nil
					p.committed.Broadcast()

				case canceled_reply:
					p.begin.Signal()
				}
			}()

			switch reply {
			case engaged_reply:
				atomic.AddUint32(&p.stats.Send, 1)

			case canceled_reply:
				atomic.AddUint32(&p.stats.Cancel, 1)

			default:
				if err == nil {
					err = errors.New("bad reply to commit command")
				}

				p.log(err)
				atomic.AddUint32(&p.stats.Error, 1)
				return
			}
		} else {
			if commanded {
				atomic.AddUint32(&p.stats.Rollback, 1)
			} else {
				atomic.AddUint32(&p.stats.Error, 1)
				return
			}
		}
	}
}

func (p *pusher) initial_error(err error) {
	soft := false

	if err == io.EOF {
		soft = true
	} else if operr, ok := err.(*net.OpError); ok && operr.Err == syscall.EPIPE {
		soft = true
	}

	if !soft {
		p.log(err)
		atomic.AddUint32(&p.stats.Error, 1)
	}
}

func (p *pusher) log(err error) {
	if p.logger != nil {
		p.logger(err)
	}
}

func (p *pusher) io_tick() {
	for _ = range p.ticker.C {
		p.begin.Signal()
	}
}

func (p *pusher) Stats() *Stats {
	return &Stats{
		atomic.LoadUint32(&p.stats.Queue),
		atomic.LoadUint32(&p.stats.Send),
		atomic.LoadUint32(&p.stats.Error),
		atomic.LoadUint32(&p.stats.Rollback),
		atomic.LoadUint32(&p.stats.Cancel),
	}
}

func (s *Stats) String() string {
	return fmt.Sprintf("queue=%v send=%v error=%v rollback=%v cancel=%v",
		s.Queue, s.Send, s.Error, s.Rollback, s.Cancel)
}
