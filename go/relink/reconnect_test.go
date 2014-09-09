package relink

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestReconnect(t *testing.T) {
	faultListener, err := FaultListen("tcp", "localhost:12346")
	if err != nil {
		t.Fatal(err)
	}

	connector := &NetConnector{
		Network: "tcp",
		Address: "localhost:12346",
	}

	listenEndpoint := Endpoint{
		Links:  make(chan *Link),
		Logger: TestLogger,
	}

	if err := listenEndpoint.Listen(faultListener, ""); err != nil {
		t.Fatal(err)
	}

	connectEndpoint := Endpoint{
		Logger: TestLogger,
	}

	link, err := connectEndpoint.Connect(connector, "")
	if err != nil {
		t.Fatal(err)
	}

	var received [][]byte
	exited := make(chan bool)

	go func() {
		defer func() {
			println("receiver: exiting")
			exited <- true
		}()

		links := listenEndpoint.Links

		var link *Link
		var messages <-chan *IncomingMessage
		var errors <-chan error

		for link == nil || messages != nil || errors != nil {
			select {
			case l := <-links:
				if l == nil {
					println("receiver: links channel closed")
					links = nil
					break
				}

				if link != nil {
					println("receiver: another link open")
					break
				}

				println("receiver: link open")

				link = l
				messages = link.IncomingMessages
				errors = link.Errors

			case im := <-messages:
				if im == nil {
					println("receiver: message channel closed")
					listenEndpoint.Close()
					messages = nil
					break
				}

				received = append(received, im.Consume()[0])

			case err := <-errors:
				if err != nil {
					t.Error("receiver: link lost")
				} else {
					println("receiver: link lost channel closed")
					errors = nil
				}

			}

			time.Sleep(1)
		}
	}()

	lastSent := -1
	timer := time.NewTimer(time.Second * 5)

loop:
	for i := 0; ; i++ {
		select {
		case <-timer.C:
			break loop

		default:
		}

		buf := make([]byte, binary.MaxVarintLen64)
		n := binary.PutVarint(buf, int64(i))
		buf = buf[:n]

		deadline := time.Now().Add(time.Second)

		if sent, err := link.Send(Message{buf}, false, deadline); err != nil {
			if sent {
				t.Error(err, "(sent)")
			} else {
				t.Error(err, "(not sent)")
			}
		}

		lastSent = i

		time.Sleep(1)
	}

	println("sender: closing")

	connectEndpoint.Close()

	<-exited

	if len(received) != int(lastSent)+1 {
		t.Errorf("wrong number of received messages (%v / %v)", len(received), int(lastSent)+1)
	} else {
		println(len(received), "messages transferred")
	}

	for i := 0; i <= lastSent && i < len(received); i++ {
		if n, err := binary.Varint(received[i]); err <= 0 {
			t.Errorf("failed to parse message: %v", received[i])
		} else if n != int64(i) {
			t.Errorf("unexpected message #%v: %v", i, n)
		}
	}
}
