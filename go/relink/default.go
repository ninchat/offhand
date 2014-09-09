package relink

import (
	"time"
)

// Default values for Binder and Endpoint options (used when zero-initialized).
const (
	DefaultMaxMulticastCount   = 1024 * 1024
	DefaultMaxMessageLength    = 1024
	DefaultMaxMessagePartSize  = 1024 * 1024 * 1024
	DefaultMaxMessageTotalSize = 1024 * 1024 * 1024

	DefaultMessageSendWindow        = 65536
	DefaultMessageReceiveBufferSize = DefaultMessageSendWindow
	DefaultMessageAckWindow         = DefaultMessageSendWindow / 4
	DefaultMessageAckDelay          = time.Second / 10

	DefaultHandshakeTimeout     = time.Second * 10
	DefaultPacketReceiveTimeout = time.Second * 30
	DefaultPacketSendTimeout    = time.Second * 30

	DefaultPingInterval = time.Second * 11
	DefaultPongTimeout  = time.Second * 7

	DefaultMaxReconnectDelay = time.Second * 60

	DefaultLinkTimeout = time.Duration(-1)
)
