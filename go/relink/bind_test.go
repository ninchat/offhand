package relink

import (
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"
)

func TestBinder(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	network := "unix"
	address := tempdir + "/socket"

	listener, err := net.Listen(network, address)
	if err != nil {
		t.Fatal(err)
	}

	binder := &Binder{
		Listener: listener,
		Logger:   TestLogger,
	}

	defer binder.Close()

	bindEndpoint1 := &Endpoint{
		Links:  make(chan *Link),
		Logger: TestLogger,
	}

	if err = bindEndpoint1.Bind(binder, "foo"); err != nil {
		t.Fatal(err)
	}

	defer bindEndpoint1.Close()

	bindEndpoint2 := &Endpoint{
		Links:  make(chan *Link),
		Logger: TestLogger,
	}

	if err = bindEndpoint2.Bind(binder, "bar"); err != nil {
		t.Fatal(err)
	}

	defer bindEndpoint2.Close()

	connector := &NetConnector{
		Network: network,
		Address: address,
	}

	connectEndpoint := &Endpoint{
		Logger: TestLogger,
	}

	link1, err := connectEndpoint.Connect(connector, "foo")
	if err != nil {
		t.Fatal(err)
	}

	defer link1.Close()

	link2, err := connectEndpoint.Connect(connector, "bar")
	if err != nil {
		t.Fatal(err)
	}

	defer link2.Close()

	link3, err := connectEndpoint.Connect(connector, "baz")
	if err != nil {
		t.Fatal(err)
	}

	defer link3.Close()

	if _, err := link1.Send(Message{[]byte("Foo")}, false, time.Time{}); err != nil {
		t.Fatal(err)
	}

	if _, err := link2.Send(Message{[]byte("Bar")}, false, time.Time{}); err != nil {
		t.Fatal(err)
	}

	go func() {
		if _, err := link3.Send(Message{[]byte("Baz")}, true, time.Time{}); err != nil {
			t.Fatal(err)
		}

		t.Fatal("reached the unreachable")
	}()

	message1 := <-(<-bindEndpoint1.Links).IncomingMessages
	message2 := <-(<-bindEndpoint2.Links).IncomingMessages

	if string(message1.Consume()[0]) != "Foo" {
		t.Fatal("unexpected first message")
	}

	if string(message2.Consume()[0]) != "Bar" {
		t.Fatal("unexpected second message")
	}
}
