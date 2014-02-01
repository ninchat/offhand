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

const (
	pusher_queue_length = 100
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

type item struct {
	payload    [][]byte
	start_time time.Time
}

type pusher struct {
	listener net.Listener
	logger   func(error)
	queue    chan *item
	unsent   int32
	mutex    sync.Mutex
	flush    *sync.Cond
	closed   bool
	stats    Stats
}

func NewListenPusher(listener net.Listener, logger func(error)) Pusher {
	p := &pusher{
		listener: listener,
		logger:   logger,
		queue:    make(chan *item, pusher_queue_length),
	}

	p.flush = sync.NewCond(&p.mutex)

	go p.accept_loop()

	return p
}

func (p *pusher) Close() {
	p.mutex.Lock()
	for atomic.LoadInt32(&p.unsent) > 0 {
		p.flush.Wait()
	}
	p.mutex.Unlock()

	close(p.queue)
	p.closed = true
	p.listener.Close()
}

func (p *pusher) SendMultipart(message [][]byte, start_time time.Time) (err error) {
	var payload_size uint64
	payload := make([][]byte, 1 + len(message) * 2)

	for i, frame_data := range message {
		frame_size := len(frame_data)
		frame_head := make([]byte, 4)
		binary.LittleEndian.PutUint32(frame_head, uint32(frame_size))

		payload[1 + i * 2 + 0] = frame_head
		payload[1 + i * 2 + 1] = frame_data

		payload_size += uint64(len(frame_head) + frame_size)
	}

	if payload_size > 0xffffffff {
		err = errors.New("message too long")
		return
	}

	payload[0] = make([]byte, 4)
	binary.LittleEndian.PutUint32(payload[0], uint32(payload_size))

	atomic.AddInt32(&p.unsent, 1)

	p.queue<- &item{
		payload:    payload,
		start_time: start_time,
	}

	atomic.AddUint32(&p.stats.Queue, 1)

	return
}

func (p *pusher) accept_loop() {
	for {
		conn, err := p.listener.Accept()

		if p.closed {
			return
		}

		if err == nil {
			go p.conn_loop(conn)
		}
	}
}

func (p *pusher) conn_loop(conn net.Conn) {
	disable_linger := true

	defer func() {
		if disable_linger {
			if tcp := conn.(*net.TCPConn); tcp != nil {
				tcp.SetLinger(0)
			}
		}

		conn.Close()
	}()

	for {
		keepalive_timer := time.NewTimer(keepalive_interval)

		select {
		case item := <-p.queue:
			keepalive_timer.Stop()

			if item == nil {
				disable_linger = false
				return
			}

			if !p.send_item(conn, item) {
				conn.Close()
				conn = nil
			}

			if item.payload != nil {
				p.queue<- item
			}

			if conn == nil {
				return
			}

		case <-keepalive_timer.C:
			conn.SetDeadline(time.Now().Add(keepalive_timeout))

			if _, err := conn.Write([]byte{ keepalive_command }); err != nil {
				p.initial_error(err)
				return
			}

			buf := make([]byte, 1)

			_, err := conn.Read(buf)
			if err == nil && buf[0] != keepalive_reply {
				err = errors.New("bad reply to keepalive command")
			}
			if err != nil {
				p.initial_error(err)
				return
			}
		}
	}
}

func (p *pusher) send_item(conn net.Conn, item *item) (ok bool) {
	buf := make([]byte, 1)

	conn.SetDeadline(time.Now().Add(begin_timeout))

	if _, err := conn.Write([]byte{ begin_command }); err != nil {
		p.initial_error(err)
		return
	}

	for _, buf := range item.payload {
		if _, err := conn.Write(buf); err != nil {
			p.initial_error(err)
			return
		}
	}

	_, err := conn.Read(buf)
	if err == nil && buf[0] != received_reply {
		err = errors.New("bad reply to begin command")
	}
	if err != nil {
		p.initial_error(err)
		return
	}

	conn.SetDeadline(time.Now().Add(commit_timeout))

	commanded := false

	if _, err := conn.Write([]byte{ commit_command }); err != nil {
		p.log(err)
	} else {
		latency := uint32(time.Now().Sub(item.start_time).Nanoseconds() / 1000)
		if binary.Write(conn, binary.LittleEndian, &latency) == nil {
			commanded = true
		}
	}

	reply := no_reply

	if commanded {
		_, err = conn.Read(buf)
		if err == nil {
			reply = buf[0]
		}
	}

	switch reply {
	case engaged_reply:
		atomic.AddUint32(&p.stats.Send, 1)
		item.payload = nil
		ok = true

		if atomic.AddInt32(&p.unsent, -1) == 0 {
			p.flush.Broadcast()
		}

	case canceled_reply:
		atomic.AddUint32(&p.stats.Cancel, 1)
		ok = true

	default:
		if err == nil {
			err = errors.New("bad reply to commit command")
		}

		p.log(err)
		atomic.AddUint32(&p.stats.Error, 1)
	}

	return
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
