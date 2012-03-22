package offhand

import (
	"fmt"
	"net"
	"testing"
)

func new_listener(t *testing.T, n int) (l net.Listener) {
	l, err := net.Listen("unix", fmt.Sprintf(".test/sockets/%d", n))
	if err != nil {
		t.Fatalf("listen %d: %s", n, err.Error())
	}
	return
}

func new_pusher(t *testing.T, n int) Pusher {
	return NewListenPusher(new_listener(t, n))
}

func send(t *testing.T, p Pusher, n int, message [][]byte) {
	err := p.SendMultipart(message)
	if err != nil {
		t.Errorf("send %d: %s", n, err.Error())
	}
}

func close_with_stats(t *testing.T, n int, p Pusher) {
	p.Close()
	t.Log("pusher", n, "stats:", p.Stats().String())
}

func push(t *testing.T, exit func(), n int, message [][]byte) {
	p := new_pusher(t, n)
	send(t, p, n, message)
	close_with_stats(t, n, p)
	exit()
}

func TestSequence(t *testing.T) {
	p := new_pusher(t, 0)

	send(t, p, 0, [][]byte{ []byte{ 0xf0, 0x00, 0x0d } })
	send(t, p, 0, [][]byte{})
	send(t, p, 0, [][]byte{ []byte{}, []byte{ 0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba }, []byte{}, []byte{}, []byte{ 0xbe }, []byte{} })

	close_with_stats(t, 0, p)

	if p.SendMultipart([][]byte{}) == nil {
		t.Error("send succeeded after close")
	}

	close_with_stats(t, 0, new_pusher(t, 0))
	close_with_stats(t, 1, new_pusher(t, 1))
}

func TestParallel(t *testing.T) {
	done := make(chan bool)
	exit := func() { done<- true }

	go push(t, exit, 0, [][]byte{ []byte{ 0xff, 0xee, 0xdd } })
	go push(t, exit, 1, [][]byte{})
	go push(t, exit, 2, [][]byte{ []byte{}, []byte{ 0xcc, 0xcc, 0xcc, 0xbb, 0xbb, 0xbb, 0xaa }, []byte{}, []byte{}, []byte{ 0xaa }, []byte{} })

	<-done
	<-done
	<-done

	p0 := new_pusher(t, 0)
	p1 := new_pusher(t, 1)
	p2 := new_pusher(t, 2)
	close_with_stats(t, 2, p2)
	close_with_stats(t, 0, p0)
	close_with_stats(t, 1, p1)
}
