package offhand

import (
	"net"
	"time"
)

const (
	keepaliveInterval = time.Duration(41.235e9)

	starttlsTimeout  = time.Second * 50
	beginTimeout     = time.Second * 10
	commitTimeout    = time.Second * 30
	rollbackTimeout  = time.Second * 70
	keepaliveTimeout = time.Second * 20

	starttlsCommand  = byte(0)
	beginCommand     = byte(10)
	commitCommand    = byte(21)
	rollbackCommand  = byte(30)
	keepaliveCommand = byte(40)

	noReply        = byte(0)
	receivedReply  = byte(11)
	engagedReply   = byte(21)
	canceledReply  = byte(22)
	keepaliveReply = byte(41)
)

func timeout(err error) bool {
	operr, ok := err.(*net.OpError)
	return ok && operr.Timeout()
}
