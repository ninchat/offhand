package relink

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	handshakeBufferSize = (1 + 0) + (1 + 1 + 1) + 4 + (8 + 8)
)

func writeHandshakeVersion(w io.Writer) (err error) {
	_, err = w.Write([]byte{ProtocolVersion, 0, 0, 0, 0, 0, 0, 0})
	return
}

func readHandshakeVersion(r io.Reader) (err error) {
	buf := make([]byte, 8)

	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}

	if buf[0] != uint8(ProtocolVersion) {
		err = fmt.Errorf("unsupported protocol version %d", buf[0])
	}
	return
}

func writeHandshakeEndpoint(w io.Writer, name []byte) (err error) {
	if _, err = w.Write([]byte{byte(len(name))}); err != nil {
		return
	}

	_, err = w.Write(name)
	return
}

func readHandshakeEndpoint(r io.Reader) (name []byte, err error) {
	length := make([]byte, 1)

	if _, err = io.ReadFull(r, length); err != nil {
		return
	}

	name = make([]byte, length[0])
	_, err = io.ReadFull(r, name)
	return
}

// handshakeConfig
type handshakeConfig struct {
	ConnectorChannelIdSize uint8
	ListenerChannelIdSize  uint8
	Flags                  uint8
}

func writeHandshakeConfig(w *alignWriter, connector, listener ChannelOptions, flags uint8) (err error) {
	config := handshakeConfig{
		ConnectorChannelIdSize: connector.idSize(),
		ListenerChannelIdSize:  listener.idSize(),
		Flags: flags,
	}

	if err = binary.Write(w, binary.LittleEndian, config); err != nil {
		return
	}

	return w.Align(8)
}

func readHandshakeConfig(r *alignReader) (config handshakeConfig, err error) {
	if err = binary.Read(r, binary.LittleEndian, &config); err != nil {
		return
	}

	err = r.Align(8)
	return
}

// handshakeIdent
type handshakeIdent struct {
	Epoch  int64
	LinkId linkId
}

func writeHandshakeIdent(w io.Writer, epoch int64, linkId linkId) error {
	ident := handshakeIdent{
		Epoch:  epoch,
		LinkId: linkId,
	}

	return binary.Write(w, binary.LittleEndian, ident)
}

func readHandshakeIdent(r io.Reader) (ident handshakeIdent, err error) {
	err = binary.Read(r, binary.LittleEndian, &ident)
	return
}
