package relink

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"testing"
	"time"
)

func TestTLS(t *testing.T) {
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
	}

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}

	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{
					certificate,
				},
				PrivateKey: key,
			},
		},
		InsecureSkipVerify: true,
	}

	listenEndpoint := Endpoint{
		Links: make(chan *Link),
	}

	listener, err := tls.Listen("tcp", "localhost:12345", config)
	if err != nil {
		t.Fatal(err)
	}

	if err := listenEndpoint.Listen(listener, ""); err != nil {
		t.Fatal(err)
	}

	connectEndpoint := Endpoint{}

	connector := &TLSConnector{
		NetConnector: NetConnector{
			Network: "tcp",
			Address: "localhost:12345",
		},
		Config: config,
	}

	link, err := connectEndpoint.Connect(connector, "")
	if err != nil {
		t.Fatal(err)
	}

	if sent, err := link.Send(Message{[]byte("hello")}, false, time.Time{}); err != nil {
		t.Fatal(err)
	} else if !sent {
		t.Fatal("not sent")
	}

	links := listenEndpoint.Links
	var messages <-chan *IncomingMessage
	var message Message

	for links != nil || messages != nil {
		select {
		case link := <-links:
			if link != nil {
				messages = link.IncomingMessages
			}
			links = nil

		case im := <-messages:
			messages = nil
			message = im.Consume()
		}
	}

	go connectEndpoint.Close()
	listenEndpoint.Close()

	if message == nil {
		t.Fatal("no message")
	}

	if string(message[0]) != "hello" {
		t.Fatal("incorrect message")
	}
}
