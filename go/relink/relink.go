// Package relink implements the Relink protocol version 1.
package relink

// This file contains miscellaneous functions that don't fit elsewhere.

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

// jitter randomizes a timeout by increasing or decreasing it.
func jitter(d time.Duration, scale float64) time.Duration {
	return time.Duration(float64(d) + (float64(d) * scale * rand.Float64()))
}

// deadline converts a timeout to a randomized deadline.
func deadline(timeout time.Duration) (deadline time.Time) {
	if timeout >= 0 {
		deadline = time.Now().Add(jitter(timeout, 0.25))
	}
	return
}

// connString tries to return a proper description for a network connection.
func connString(c net.Conn) string {
	var x interface{} = c

	if _, ok := c.(fmt.Stringer); !ok {
		if addr := c.RemoteAddr(); addr != nil {
			x = addr
		}
	}

	return fmt.Sprint(x)
}
