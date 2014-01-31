package offhand

import (
	"time"
)

const (
	keepalive_interval = time.Duration(41.235e9)

	begin_timeout      = time.Duration(10e9)
	commit_timeout     = time.Duration(30e9)
	keepalive_timeout  = time.Duration(10e9)

	begin_command      = byte(10)
	oldcommit_command  = byte(20)
	commit_command     = byte(21)
	rollback_command   = byte(30)
	keepalive_command  = byte(40)

	no_reply           = byte( 0)
	received_reply     = byte(11)
	engaged_reply      = byte(21)
	canceled_reply     = byte(22)
	keepalive_reply    = byte(41)
)
