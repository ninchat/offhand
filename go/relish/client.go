package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"../relink"
)

func Client() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s client host:port\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	connector := &relink.TLSConnector{
		NetConnector: relink.NetConnector{
			Network: "tcp",
			Address: flag.Arg(1),
			Dialer: net.Dialer{
				Timeout: time.Second,
			},
		},
		Config: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	endpoint := &relink.Endpoint{
		Links:  make(chan *relink.Link),
		Logger: relink.NullLogger,
	}

	defer endpoint.Close()

	if _, err := endpoint.Connect(connector, "relish"); err != nil {
		log.Print(err)
		os.Exit(1)
	}

	link := <-endpoint.Links
	messages := link.IncomingMessages
	errors := link.Errors
	stdin := make(chan []byte)

	go func() {
		defer close(stdin)

		r := bufio.NewReader(os.Stdin)

		for {
			line, err := r.ReadBytes('\n')

			if len(line) > 0 {
				stdin <- line
			}

			if err != nil {
				log.Print(err)
				break
			}
		}
	}()

	var fail bool

	for messages != nil || errors != nil {
		select {
		case line := <-stdin:
			if line == nil {
				stdin = nil
				if link != nil {
					link.Close()
				}
				break
			}

			if link != nil {
				if _, err := link.Send([][]byte{line}, false, time.Time{}); err != nil {
					log.Print(err)
					link.Close()
					link = nil
				}
			}

		case im := <-messages:
			if im == nil {
				messages = nil
				if link != nil {
					link.Close()
				}
				break
			}

			if _, err := os.Stdout.Write(im.Consume()[0]); err != nil {
				log.Print(err)
				os.Exit(1)
			}

		case err := <-errors:
			if err != nil {
				log.Print(err)
				fail = true
			}

			errors = nil
		}
	}

	if fail {
		os.Exit(255)
	}
}
