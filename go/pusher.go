package offhand

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	pusherQueueLength = 100
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
	SendMultipart(ctx context.Context, message [][]byte, startTime time.Time) (bool, error)
	Close()
	LoadStats(s *Stats)
}

type item struct {
	ctx       context.Context
	data      []byte
	startTime time.Time
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
		queue:    make(chan *item, pusherQueueLength),
	}

	p.flush = sync.NewCond(&p.mutex)

	go p.acceptLoop()

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

func (p *pusher) SendMultipart(ctx context.Context, message [][]byte, startTime time.Time) (ok bool, err error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.closing {
		return
	}

	var messageSize uint32

	for _, frame := range message {
		messageSize += uint32(4 + len(frame))
	}

	data := make([]byte, 5+messageSize)
	data[0] = beginCommand
	binary.LittleEndian.PutUint32(data[1:5], messageSize)

	pos := data[5:]

	for _, frame := range message {
		binary.LittleEndian.PutUint32(pos[:4], uint32(len(frame)))
		pos = pos[4:]

		copy(pos, frame)
		pos = pos[len(frame):]
	}

	atomic.AddInt32(&p.Queued, 1)

	select {
	case p.queue <- &item{ctx, data, startTime}:
		ok = true

	case <-ctx.Done():
		if atomic.AddInt32(&p.Queued, -1) == 0 {
			p.flush.Broadcast()
		}

		err = ctx.Err()
	}

	return
}

func (p *pusher) acceptLoop() {
	for {
		conn, err := p.listener.Accept()

		if p.closed {
			return
		}

		if err == nil {
			go p.connLoop(conn)
		}
	}
}

func (p *pusher) connLoop(conn net.Conn) {
	atomic.AddInt32(&p.Conns, 1)
	defer atomic.AddInt32(&p.Conns, -1)

	disableLinger := true

	defer func() {
		if disableLinger {
			if tcp, ok := conn.(*net.TCPConn); ok {
				tcp.SetLinger(0)
			}
		}

		conn.Close()
	}()

	replyBuf := make([]byte, 1)

	for {
		keepaliveTimer := time.NewTimer(keepaliveInterval)

		select {
		case item := <-p.queue:
			keepaliveTimer.Stop()

			if item == nil {
				disableLinger = false
				return
			}

			select {
			case <-item.ctx.Done():
				if atomic.AddInt32(&p.Queued, -1) == 0 {
					p.flush.Broadcast()
				}

			default:
				if !p.sendItem(conn, item) {
					return
				}
			}

		case <-keepaliveTimer.C:
			conn.SetDeadline(time.Now().Add(keepaliveTimeout))

			if _, err := conn.Write([]byte{keepaliveCommand}); err != nil {
				p.logInitial(err)
				return
			}

			if _, err := conn.Read(replyBuf); err != nil {
				p.logInitial(err)
				return
			}

			if replyBuf[0] != keepaliveReply {
				p.log(errors.New("bad reply to keepalive command"))
				return
			}
		}
	}
}

func (p *pusher) sendItem(conn net.Conn, item *item) (ok bool) {
	replyBuf := make([]byte, 1)
	rollback := false

	// begin command + message

	conn.SetDeadline(time.Now().Add(beginTimeout))

	if n, err := conn.Write(item.data); err != nil {
		if rollback {
			return
		}

		p.queue <- item
		p.logInitial(err)

		if !timeout(err) {
			return
		}

		rollback = true
		conn.SetDeadline(time.Now().Add(rollbackTimeout))

		if _, err := conn.Write(item.data[n:]); err != nil {
			return
		}
	}

	// received reply

	for _, err := conn.Read(replyBuf); err != nil; {
		if rollback {
			return
		}

		p.queue <- item
		p.logInitial(err)

		if !timeout(err) {
			return
		}

		rollback = true
		conn.SetDeadline(time.Now().Add(rollbackTimeout))
	}

	if replyBuf[0] != receivedReply {
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
			conn.SetDeadline(time.Now().Add(rollbackTimeout))

		default:
		}
	}

	// rollback command

	if rollback {
		if _, err := conn.Write([]byte{rollbackCommand}); err != nil {
			return
		}

		ok = true
		return
	}

	// commit command

	conn.SetDeadline(time.Now().Add(commitTimeout))

	commitBuf := make([]byte, 5)
	commitBuf[0] = commitCommand
	binary.LittleEndian.PutUint32(commitBuf[1:], uint32(time.Now().Sub(item.startTime).Nanoseconds()/1000))

	if _, err := conn.Write(commitBuf); err != nil {
		p.queue <- item
		p.log(err)
		return
	}

	// commit reply

	if _, err := conn.Read(replyBuf); err != nil {
		p.queue <- item
		p.log(err)
		return
	}

	switch replyBuf[0] {
	case engagedReply:
		if atomic.AddInt32(&p.Queued, -1) == 0 {
			p.flush.Broadcast()
		}

		atomic.AddUint64(&p.TotalDelayUs, uint64(time.Now().Sub(item.startTime).Nanoseconds())/1000)
		atomic.AddUint64(&p.TotalSent, 1)
		ok = true

	case canceledReply:
		p.queue <- item
		atomic.AddUint64(&p.TotalCancelled, 1)
		ok = true

	default:
		p.queue <- item
		p.log(errors.New("bad reply to commit command"))
	}

	return
}

func (p *pusher) logInitial(err error) {
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
