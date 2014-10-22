package relink

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"testing"
	"time"
)

func TestEndpointPairWithChannels(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	addr := tempdir + "/socket"
	tick := time.NewTicker(time.Second).C
	exit := make(chan bool)

	var connectStats Stats
	var listenStats Stats

	go func() {
		e := Endpoint{
			IncomingOptions: ChannelOptions{
				IdSize: 8,
			},
			Links:            make(chan *Link, 1),
			Logger:           TestLogger,
			Stats:            &listenStats,
			MessageAckWindow: 1,
		}

		os.Remove(addr)

		l, err := net.Listen("unix", addr)
		if err != nil {
			t.Fatal(err)
		}

		err = e.Listen(l, "")
		if err != nil {
			t.Fatal(err)
		}

		links := e.Links

		var channels <-chan *IncomingChannel
		var errors <-chan error
		var messages <-chan *IncomingMessage

		var link *Link

		for links != nil || channels != nil || errors != nil || messages != nil {
			select {
			case l := <-links:
				if l != nil {
					log.Print("listen: link open")

					link = l
					channels = link.IncomingChannels
					errors = link.Errors
				} else {
					log.Print("listen: endpoint links channel closed")
					links = nil
				}

			case err := <-errors:
				if err != nil {
					log.Print("listen: link lost")
				} else {
					log.Print("listen: link lost channel closed")
					e.Close()
					errors = nil
				}

			case c := <-channels:
				if c == nil {
					log.Print("listen: link channels channel closed")
					link.Close()
					channels = nil
					break
				}

				log.Print("listen: incoming channel: ", c.Id)
				messages = c.IncomingMessages

			case im := <-messages:
				if im == nil {
					log.Print("listen: message channel closed")
					messages = nil
					break
				}

				log.Print("listen: message received: ", im.Consume())
			}
		}

		log.Print("listen: exiting")

		exit <- true
	}()

	go func() {
		e := Endpoint{
			OutgoingOptions: ChannelOptions{
				IdSize: 8,
			},
			Logger:            TestLogger,
			Stats:             &connectStats,
			MessageSendWindow: 1,
		}

		c := &NetConnector{
			Network: "unix",
			Address: addr,
		}

		l, err := e.Connect(c, "")
		if err != nil {
			t.Fatal(err)
		}

		channel := l.OutgoingChannel(MakeChannelId(uint64(555)))

		var deadline time.Time

		for i := 0; i <= 255; i++ {
			log.Print("connect: ", i, " sending...")

			if _, err := channel.Send([][]byte{{byte(i)}}, false, deadline); err != nil {
				t.Fatal(err)
			}

			log.Print("connect: ", i, " sent")
		}

		log.Print("connect: loop done")

		e.Close()

		errors := l.Errors
		messages := l.IncomingMessages

		select {
		case err := <-errors:
			if err != nil {
				log.Print("connect: link lost")
			} else {
				log.Print("connect: link lost channel closed")
			}

			errors = nil

		case im := <-messages:
			if im == nil {
				log.Print("connect: message channel closed")
				messages = nil
				break
			}

			log.Print("connect: message received: ", im.Consume())
		}

		log.Print("connect: exiting")

		exit <- true
	}()

	for exited := 0; exited < 2; {
		select {
		case <-tick:
			log.Printf("[ CONNECT STATS ]  SentPackets:%4d, SentMessages:%4d, ReceivedPackets:%4d, ReceivedMessages:%4d\n[ LISTEN  STATS ]  SentPackets:%4d, SentMessages:%4d, ReceivedPackets:%4d, ReceivedMessages:%4d", connectStats.SentPackets, connectStats.SentMessages, connectStats.ReceivedPackets, connectStats.ReceivedMessages, listenStats.SentPackets, listenStats.SentMessages, listenStats.ReceivedPackets, listenStats.ReceivedMessages)

		case <-exit:
			exited++
		}
	}
}

func TestEndpointPairWithoutChannels(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	addr := tempdir + "/socket"
	tick := time.NewTicker(time.Second).C
	exit := make(chan bool)

	var dialStats Stats
	var listenStats Stats

	go func() {
		e := Endpoint{
			Links:             make(chan *Link),
			Logger:            TestLogger,
			Stats:             &listenStats,
			MessageSendWindow: 50,
		}

		os.Remove(addr)

		l, err := net.Listen("unix", addr)
		if err != nil {
			t.Fatal(err)
		}

		err = e.Listen(l, "")
		if err != nil {
			t.Fatal(err)
		}

		link := <-e.Links

		count := 0

		for im := range link.IncomingMessages {
			log.Print("listen: message received: ", im.Consume())

			count++
			if count == 256 {
				e.Close()
			}
		}

		exit <- true
	}()

	go func() {
		e := Endpoint{
			Logger:            TestLogger,
			Stats:             &dialStats,
			MessageSendWindow: 100,
		}

		d := &NetConnector{
			Network: "unix",
			Address: addr,
		}

		link, err := e.Connect(d, "")
		if err != nil {
			t.Fatal(err)
		}

		var deadline time.Time

		for i := 0; i <= 255; i++ {
			log.Print("dial: ", i, " sending...")

			if _, err := link.Send([][]byte{{byte(i)}}, false, deadline); err != nil {
				t.Fatal(err)
			}

			log.Print("dial: ", i, " sent")
		}

		e.Close()

		exit <- true
	}()

	for exited := 0; exited < 2; {
		select {
		case <-tick:
			log.Printf("[ DIAL   STATS ]  SentPackets:%4d, SentMessages:%4d, ReceivedPackets:%4d, ReceivedMessages:%4d\n[ LISTEN STATS ]  SentPackets:%4d, SentMessages:%4d, ReceivedPackets:%4d, ReceivedMessages:%4d", dialStats.SentPackets, dialStats.SentMessages, dialStats.ReceivedPackets, dialStats.ReceivedMessages, listenStats.SentPackets, listenStats.SentMessages, listenStats.ReceivedPackets, listenStats.ReceivedMessages)

		case <-exit:
			exited++
		}
	}
}

func testGeneralPacketHeader(packetType uint8) (header byte) {
	header = packetType << 5
	return
}

func testChannelPacketHeader(multicast bool, format, x uint8) (header byte) {
	header = (1 << 0) | (format << 2) | (x << 5)
	if multicast {
		header |= 1 << 1
	}
	return
}

// testReceiveConn
type testReceiveConn struct {
	reader *bytes.Reader
	term   chan bool
}

func newTestReceiveConn(data []byte) (c net.Conn, err error) {
	c = &testReceiveConn{
		reader: bytes.NewReader(data),
		term:   make(chan bool),
	}
	return
}

func (c *testReceiveConn) Read(b []byte) (n int, err error) {
	n, err = c.reader.Read(b)
	if n == 0 {
		<-c.term
		if err == nil {
			err = io.EOF
		}
	}
	log.Print("Read:  ", b[:n])
	return
}

func (c *testReceiveConn) Write(b []byte) (n int, err error) {
	log.Print("Write: ", b)
	n = len(b)
	return
}

func (c *testReceiveConn) Close() (err error) {
	close(c.term)
	return
}

func (c *testReceiveConn) LocalAddr() (addr net.Addr) {
	return
}

func (c *testReceiveConn) RemoteAddr() (addr net.Addr) {
	return
}

func (c *testReceiveConn) SetDeadline(t time.Time) (err error) {
	return
}

func (c *testReceiveConn) SetReadDeadline(t time.Time) (err error) {
	return
}

func (c *testReceiveConn) SetWriteDeadline(t time.Time) (err error) {
	return
}

func (c *testReceiveConn) String() string {
	return "test-conn"
}

// testReceiveDialer
type testReceiveDialer struct {
	data []byte
}

func newTestReceiveDialer(data []byte) *testReceiveDialer {
	return &testReceiveDialer{
		data: data,
	}
}

func (d *testReceiveDialer) Close() (err error) {
	return
}

func (d *testReceiveDialer) Connect() (c net.Conn, err error) {
	if d.data != nil {
		c, err = newTestReceiveConn(d.data)
		d.data = nil
	} else {
		err = io.EOF
	}
	return
}

func (d *testReceiveDialer) String() string {
	return "test-dialer"
}

func TestDialEndpointReceive(t *testing.T) {
	data := []byte{
		/* version     */ 0,
		/* (padding)   */ 0, 0, 0, 0, 0, 0, 0,
		/* new epoch   */ 9, 8, 7, 6, 5, 4, 3, 2,
		/* new link id */ 1, 0, 0, 0, 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralNop), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralPing), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(false, protocolFormatMessage, 0), 1,
		/* channel   */ 69, 96,
		/* size 0    */ 1, 0,
		/* (padding) */ 0, 0,
		/* data 0    */ 42,
		/* (padding) */ 0, 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(false, protocolFormatMessage, protocolMessageFlagLong|protocolMessageFlagLarge), 0,
		/* channel   */ 69, 96,
		/* parts     */ 2, 0, 0, 0,
		/* size 0    */ 1, 0, 0, 0, 0, 0, 0, 0,
		/* size 1    */ 2, 0, 0, 0, 0, 0, 0, 0,
		/* data 0    */ 52,
		/* (padding) */ 0, 0, 0, 0, 0, 0, 0,
		/* data 1    */ 53, 35,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(true, protocolFormatMessage, 0), 1,
		/* (padding) */ 0, 0,
		/* count     */ 2, 0, 0, 0,
		/* channel 0 */ 69, 96,
		/* channel 1 */ 88, 88,
		/* size 0    */ 1, 0,
		/* (padding) */ 0, 0,
		/* data 0    */ 62,
		/* (padding) */ 0, 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(false, protocolFormatChannelOp, protocolChannelOpClose), 0,
		/* channel   */ 88, 88,
		/* (padding) */ 0, 0, 0, 0,

		testChannelPacketHeader(true, protocolFormatChannelOp, protocolChannelOpClose), 0,
		/* (padding) */ 0, 0,
		/* count     */ 2, 0, 0, 0,
		/* channel 0 */ 69, 96,
		/* channel 1 */ 69, 97,
		/* (padding) */ 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralPing), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralShutdown), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,
	}

	e := &Endpoint{
		IncomingOptions: ChannelOptions{
			IdSize: 2,
		},
		Links:  make(chan *Link),
		Logger: TestLogger,
	}

	defer e.Close()

	_, err := e.Connect(newTestReceiveDialer(data), "")
	if err != nil {
		t.Fatal(err)
	}

	testEndpoint(e)
}

// testReceiveListener
type testReceiveListener struct {
	data []byte
	term chan bool
}

func newTestReceiveListener(data []byte) *testReceiveListener {
	return &testReceiveListener{
		data: data,
		term: make(chan bool),
	}
}

func (l *testReceiveListener) Accept() (c net.Conn, err error) {
	if l.data != nil {
		c, err = newTestReceiveConn(l.data)
		l.data = nil
	} else {
		<-l.term
		err = io.EOF
	}
	return
}

func (l *testReceiveListener) Close() (err error) {
	println("closing listener")
	close(l.term)
	return
}

func (l *testReceiveListener) Addr() (a net.Addr) {
	return
}

func (l *testReceiveListener) String() string {
	return "test-listener"
}

func TestListenEndpointReceive(t *testing.T) {
	data := []byte{
		/* version                   */ 0,
		/* (padding)                 */ 0, 0, 0, 0, 0, 0, 0,
		/* endpoint name length      */ 0,
		/* connector channel id size */ 2,
		/* listener channel id size  */ 0,
		/* flags                     */ 0,
		/* (padding)                 */ 0, 0, 0, 0,
		/* no epoch                  */ 0, 0, 0, 0, 0, 0, 0, 0,
		/* no link id                */ 0, 0, 0, 0, 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralNop), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralPing), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(false, protocolFormatMessage, 0), 1,
		/* channel   */ 69, 96,
		/* size 0    */ 1, 0,
		/* (padding) */ 0, 0,
		/* data 0    */ 42,
		/* (padding) */ 0, 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(false, protocolFormatMessage, protocolMessageFlagLong|protocolMessageFlagLarge), 0,
		/* channel   */ 69, 96,
		/* length    */ 2, 0, 0, 0,
		/* size 0    */ 1, 0, 0, 0, 0, 0, 0, 0,
		/* size 1    */ 2, 0, 0, 0, 0, 0, 0, 0,
		/* data 0    */ 52,
		/* (padding) */ 0, 0, 0, 0, 0, 0, 0,
		/* data 1    */ 53, 35,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(true, protocolFormatMessage, 0), 1,
		/* (padding) */ 0, 0,
		/* count     */ 2, 0, 0, 0,
		/* channel 0 */ 69, 96,
		/* channel 1 */ 88, 88,
		/* size 0    */ 1, 0,
		/* (padding) */ 0, 0,
		/* data 0    */ 62,
		/* (padding) */ 0, 0, 0, 0, 0, 0, 0,

		testChannelPacketHeader(false, protocolFormatChannelOp, protocolChannelOpClose), 0,
		/* channel   */ 88, 88,
		/* (padding) */ 0, 0, 0, 0,

		testChannelPacketHeader(true, protocolFormatChannelOp, protocolChannelOpClose), 0,
		/* (padding) */ 0, 0,
		/* count     */ 2, 0, 0, 0,
		/* channel 0 */ 69, 96,
		/* channel 1 */ 69, 97,
		/* (padding) */ 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralPing), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,

		testGeneralPacketHeader(protocolGeneralShutdown), 0,
		/* (padding) */ 0, 0, 0, 0, 0, 0,
	}

	e := &Endpoint{
		IncomingOptions: ChannelOptions{
			IdSize: 2,
		},
		Links:  make(chan *Link),
		Logger: TestLogger,
	}

	defer e.Close()

	err := e.Listen(newTestReceiveListener(data), "")
	if err != nil {
		t.Fatal(err)
	}

	testEndpoint(e)
}

func testEndpoint(e *Endpoint) {
	poke := make(chan bool, 1)

	go func() {
		link := <-e.Links
		channel := <-link.IncomingChannels

		go func() {
			<-link.IncomingChannels
			poke <- true
		}()

		for m := range channel.IncomingMessages {
			log.Print("Message = ", m.Consume())
		}

		log.Print("Message channel closed")
		poke <- true
	}()

	<-poke
	<-poke
}
