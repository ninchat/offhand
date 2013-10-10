package offhand

import (
	"time"
)

const (
	begin_timeout    = time.Duration(10e9)
	commit_timeout   = time.Duration(60e9)
	tick_interval    = time.Duration( 3e9)

	begin_command    = byte(10)
	commit_command   = byte(20)
	rollback_command = byte(30)

	no_reply         = byte( 0)
	received_reply   = byte(11)
	engaged_reply    = byte(21)
	canceled_reply   = byte(22)
)
