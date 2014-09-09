package relink

import (
	"fmt"
	"net"
	"time"
)

// FaultConnector
type FaultConnector struct {
	NetConnector
}

func (fc *FaultConnector) Connect() (c net.Conn, err error) {
	c, err = fc.NetConnector.Connect()
	if err == nil {
		c = NewFaultConn(c)
	}
	return
}

func (fc *FaultConnector) String() string {
	return fmt.Sprintf("FaultConnector=%p", fc)
}

// FaultListener
type FaultListener struct {
	l net.Listener
}

func FaultListen(network, address string) (fl *FaultListener, err error) {
	l, err := net.Listen(network, address)
	if err == nil {
		fl = &FaultListener{l}
	}
	return
}

func (fl *FaultListener) Accept() (c net.Conn, err error) {
	c, err = fl.l.Accept()
	if err == nil {
		c = NewFaultConn(c)
	}
	return
}

func (fl *FaultListener) Close() error {
	return fl.l.Close()
}

func (fl *FaultListener) Addr() net.Addr {
	return fl.l.Addr()
}

func (fl *FaultListener) String() string {
	return fmt.Sprintf("FaultListener=%p", fl)
}

// FaultConn
type FaultConn struct {
	c net.Conn
}

func NewFaultConn(c net.Conn) (fc *FaultConn) {
	fc = &FaultConn{c}
	go fc.fault()
	return
}

func (fc *FaultConn) Read(b []byte) (int, error) {
	return fc.c.Read(b)
}

func (fc *FaultConn) Write(b []byte) (int, error) {
	return fc.c.Write(b)
}

func (fc *FaultConn) Close() error {
	return fc.c.Close()
}

func (fc *FaultConn) LocalAddr() net.Addr {
	return fc.c.LocalAddr()
}

func (fc *FaultConn) RemoteAddr() net.Addr {
	return fc.c.RemoteAddr()
}

func (fc *FaultConn) SetDeadline(t time.Time) error {
	return fc.c.SetDeadline(t)
}

func (fc *FaultConn) SetReadDeadline(t time.Time) error {
	return fc.c.SetReadDeadline(t)
}

func (fc *FaultConn) SetWriteDeadline(t time.Time) error {
	return fc.c.SetWriteDeadline(t)
}

func (fc *FaultConn) String() string {
	return fmt.Sprintf("FaultConn=%p", fc)
}

func (fc FaultConn) fault() {
	time.Sleep(jitter(time.Second*3, -0.2))
	fc.c.Close()
}
