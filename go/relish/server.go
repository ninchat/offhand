package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"
	"time"

	"../relink"
)

func Server() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s server [address]:port\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	template := &x509.Certificate{
		SerialNumber: new(big.Int),
	}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		log.Print(err)
		os.Exit(1)
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
	}

	listener, err := tls.Listen("tcp", flag.Arg(1), config)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	endpoint := &relink.Endpoint{
		Links: make(chan *relink.Link),
	}

	defer endpoint.Close()

	if err := endpoint.Listen(listener, "relish"); err != nil {
		log.Print(err)
		os.Exit(1)
	}

	for link := range endpoint.Links {
		go HandleServerLink(link)
	}
}

func HandleServerLink(link *relink.Link) {
	var stdin chan []byte
	var stdout chan []byte

	cmd := exec.Command("/bin/bash", "-i")

	stdinWriter, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
	} else {
		stdoutReader, err := cmd.StdoutPipe()
		if err != nil {
			log.Print(err)
		} else {
			cmd.Stderr = cmd.Stdout

			if err := cmd.Start(); err != nil {
				log.Print(err)
			} else {
				stdin = make(chan []byte)
				defer close(stdin)

				stdout = make(chan []byte)

				go func() {
					defer stdinWriter.Close()

					for line := range stdin {
						stdinWriter.Write(line)
					}
				}()

				go func() {
					defer close(stdout)

					for {
						buf := make([]byte, 256)
						n, err := stdoutReader.Read(buf)
						line := buf[:n]

						if len(line) > 0 {
							stdout <- line
						}

						if err != nil {
							log.Print(err)
							break
						}
					}
				}()
			}
		}
	}

	messages := link.IncomingMessages
	errors := link.Errors

	var stdinLine []byte

	for messages != nil || errors != nil || stdinLine != nil || stdout != nil {
		var stdinActive chan<- []byte
		var messagesActive <-chan *relink.IncomingMessage

		if stdinLine != nil {
			stdinActive = stdin
		} else {
			messagesActive = messages
		}

		select {
		case im := <-messagesActive:
			if im == nil {
				messages = nil
				if link != nil {
					link.Close()
				}
				break
			}

			stdinLine = im.Consume()[0]

		case err := <-errors:
			if err != nil {
				log.Print(err)
			}

			errors = nil

		case stdinActive <- stdinLine:
			stdinLine = nil

		case line := <-stdout:
			if line == nil {
				if err := cmd.Wait(); err != nil {
					log.Print(err)
				}
				stdout = nil
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
		}
	}
}
