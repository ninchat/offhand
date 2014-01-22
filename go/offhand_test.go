package offhand

import (
	"testing"
)

func Test(t *testing.T) {
	e := Endpoint{
		Outgoing: Options{IdSize: 0, Transactions: false},
		Incoming: Options{IdSize: 0, Transactions: false},
	}

	s := e.Dial("tcp", "127.0.0.1:1234")
	s.Close()
}
