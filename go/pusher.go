package offhand

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/context"
)

const (
	pusher_queue_length = 100
)

type Stats struct {
	Conns          int32
	Queued         int32
	TotalDelayUs   uint64
	TotalSent      uint64
	TotalTimeouts  uint64
	TotalErrors    uint64
	TotalCancelled uint64
}

type Pusher interface {
	SendMultipart(ctx context.Context, message [][]byte, start_time time.Time) (bool, error)
	Close()
	LoadStats(s *Stats)
}

type item struct {
	ctx        context.Context
	data       []byte
	start_time time.Time
}

type pusher struct {
	Stats

	listener net.Listener
	logger   func(error)
	queue    chan *item
	mutex    sync.RWMutex
	flush    *sync.Cond
	closing  bool
	closed   bool
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
	defer p.mutex.Unlock()

	if p.closing {
		return
	}

	p.closing = true

	for atomic.LoadInt32(&p.Queued) > 0 {
		p.flush.Wait()
	}

	close(p.queue)

	p.closed = true
	p.listener.Close()
}

func (p *pusher) SendMultipart(ctx context.Context, message [][]byte, start_time time.Time) (ok bool, err error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.closing {
		return
	}

	var message_size uint32

	for _, frame := range message {
		message_size += uint32(4 + len(frame))
	}

	data := make([]byte, 5+message_size)
	data[0] = begin_command
	binary.LittleEndian.PutUint32(data[1:5], message_size)

	pos := data[5:]

	for _, frame := range message {
		binary.LittleEndian.PutUint32(pos[:4], uint32(len(frame)))
		pos = pos[4:]

		copy(pos, frame)
		pos = pos[len(frame):]
	}

	atomic.AddInt32(&p.Queued, 1)

	select {
	case p.queue <- &item{ctx, data, start_time}:
		ok = true

	case <-ctx.Done():
		if atomic.AddInt32(&p.Queued, -1) == 0 {
			p.flush.Broadcast()
		}

		err = ctx.Err()
	}

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
	atomic.AddInt32(&p.Conns, 1)
	defer atomic.AddInt32(&p.Conns, -1)

	disable_linger := true

	defer func() {
		if disable_linger {
			if tcp := conn.(*net.TCPConn); tcp != nil {
				tcp.SetLinger(0)
			}
		}

		conn.Close()
	}()

	reply_buf := make([]byte, 1)

	for {
		keepalive_timer := time.NewTimer(keepalive_interval)

		select {
		case item := <-p.queue:
			keepalive_timer.Stop()

			if item == nil {
				disable_linger = false
				return
			}

			select {
			case <-item.ctx.Done():
				if atomic.AddInt32(&p.Queued, -1) == 0 {
					p.flush.Broadcast()
				}

			default:
				if !p.send_item(conn, item) {
					return
				}
			}

		case <-keepalive_timer.C:
			conn.SetDeadline(time.Now().Add(keepalive_timeout))

			if _, err := conn.Write([]byte{keepalive_command}); err != nil {
				p.log_initial(err)
				return
			}

			if _, err := conn.Read(reply_buf); err != nil {
				p.log_initial(err)
				return
			}

			if reply_buf[0] != keepalive_reply {
				p.log(errors.New("bad reply to keepalive command"))
				return
			}
		}
	}
}

func (p *pusher) send_item(conn net.Conn, item *item) (ok bool) {
	reply_buf := make([]byte, 1)
	rollback := false

	// begin command + message

	conn.SetDeadline(time.Now().Add(begin_timeout))

	if n, err := conn.Write(item.data); err != nil {
		if rollback {
			return
		}

		p.queue <- item
		p.log_initial(err)

		if !timeout(err) {
			return
		}

		rollback = true
		conn.SetDeadline(time.Now().Add(rollback_timeout))

		if _, err := conn.Write(item.data[n:]); err != nil {
			return
		}
	}

	// received reply

	for _, err := conn.Read(reply_buf); err != nil; {
		if rollback {
			return
		}

		p.queue <- item
		p.log_initial(err)

		if !timeout(err) {
			return
		}

		rollback = true
		conn.SetDeadline(time.Now().Add(rollback_timeout))
	}

	if reply_buf[0] != received_reply {
		if !rollback {
			p.queue <- item
			p.log(errors.New("bad reply to begin command"))
		}

		return
	}

	// check cancellation

	if !rollback {
		select {
		case <-item.ctx.Done():
			// signal Close method after writing rollback command
			defer func() {
				if atomic.AddInt32(&p.Queued, -1) == 0 {
					p.flush.Broadcast()
				}
			}()

			rollback = true
			conn.SetDeadline(time.Now().Add(rollback_timeout))

		default:
		}
	}

	// rollback command

	if rollback {
		if _, err := conn.Write([]byte{rollback_command}); err != nil {
			return
		}

		ok = true
		return
	}

	// commit command

	conn.SetDeadline(time.Now().Add(commit_timeout))

	commit_buf := make([]byte, 5)
	commit_buf[0] = commit_command
	binary.LittleEndian.PutUint32(commit_buf[1:], uint32(time.Now().Sub(item.start_time).Nanoseconds()/1000))

	if _, err := conn.Write(commit_buf); err != nil {
		p.queue <- item
		p.log(err)
		return
	}

	// commit reply

	if _, err := conn.Read(reply_buf); err != nil {
		p.queue <- item
		p.log(err)
		return
	}

	switch reply_buf[0] {
	case engaged_reply:
		if atomic.AddInt32(&p.Queued, -1) == 0 {
			p.flush.Broadcast()
		}

		atomic.AddUint64(&p.TotalDelayUs, uint64(time.Now().Sub(item.start_time).Nanoseconds())/1000)
		atomic.AddUint64(&p.TotalSent, 1)
		ok = true

	case canceled_reply:
		p.queue <- item
		atomic.AddUint64(&p.TotalCancelled, 1)
		ok = true

	default:
		p.queue <- item
		p.log(errors.New("bad reply to commit command"))
	}

	return
}

func (p *pusher) log_initial(err error) {
	soft := false

	if err == io.EOF {
		soft = true
	} else if operr, ok := err.(*net.OpError); ok && operr.Err == syscall.EPIPE {
		soft = true
	}

	if !soft {
		p.log(err)
	}
}

func (p *pusher) log(err error) {
	if p.logger != nil {
		p.logger(err)
	}

	if timeout(err) {
		atomic.AddUint64(&p.TotalTimeouts, 1)
	} else {
		atomic.AddUint64(&p.TotalErrors, 1)
	}
}

func (p *pusher) LoadStats(s *Stats) {
	s.Conns = atomic.LoadInt32(&p.Conns)
	s.Queued = atomic.LoadInt32(&p.Queued)
	s.TotalDelayUs = atomic.LoadUint64(&p.TotalDelayUs)
	s.TotalSent = atomic.LoadUint64(&p.TotalSent)
	s.TotalTimeouts = atomic.LoadUint64(&p.TotalTimeouts)
	s.TotalErrors = atomic.LoadUint64(&p.TotalErrors)
	s.TotalCancelled = atomic.LoadUint64(&p.TotalCancelled)
}
