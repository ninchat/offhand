package offhand

import (
	"time"
)

const (
	ProtocolVersion = 2
)

const (
	dial_timeout      = 15 * time.Second
	handshake_timeout = 15 * time.Second
	packet_timeout    = 45 * time.Second
	transfer_timeout  = 30 * time.Second
	reconnect_delay   =  1 * time.Second
)
