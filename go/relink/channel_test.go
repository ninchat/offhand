package relink

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func MakeChannelId(value interface{}) ChannelId {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, value); err != nil {
		panic(err)
	}
	return ChannelId(buf.Bytes())
}

func TestOutgoingChannelConsumed(t *testing.T) {
	t.Log("NO MESSAGES")

	c := newOutgoingChannel(nil, noChannelId)

	for _, n := range []uint32{0xfffffffe, 0, 1} {
		if _, err := c.onConsumed(n); err != nil {
			t.Log(err)
		} else {
			t.FailNow()
		}
	}

	t.Log("ONE MESSAGE")

	c = newOutgoingChannel(nil, noChannelId)
	c.putMessage(new(outgoingMessage), 100)
	c.getSender()

	for i := 0; i < 2; i++ {
		if enqueue, err := c.onConsumed(0); err != nil {
			t.FailNow()
		} else {
			t.Logf("sequence=%v enqueue=%v", 0, enqueue)
		}
	}

	for _, n := range []uint32{0xffffffff, 1, 2} {
		if _, err := c.onConsumed(n); err != nil {
			t.Log(err)
		} else {
			t.FailNow()
		}
	}

	t.Log("MANY MESSAGES")

	c = newOutgoingChannel(nil, noChannelId)
	for i := 0; i < 10; i++ {
		c.putMessage(new(outgoingMessage), 100)
	}
	c.getSender()

	for _, n := range []uint32{1, 4} {
		if enqueue, err := c.onConsumed(n); err != nil {
			t.FailNow()
		} else {
			t.Logf("sequence=%v enqueue=%v", n, enqueue)
		}
	}

	for _, n := range []uint32{0xffffff00, 3, 15} {
		if _, err := c.onConsumed(n); err != nil {
			t.Log(err)
		} else {
			t.FailNow()
		}
	}

	t.Log("RANGE WRAP-AROUND")

	c = newOutgoingChannel(nil, noChannelId)
	c.messageConsumed = uint32(0xffffffff - 20) // cheat
	for i := 0; i < 40; i++ {
		c.putMessage(new(outgoingMessage), 100)
	}
	c.getSender()

	for _, n := range []uint32{uint32(0x100000000 - 30), 30} {
		if _, err := c.onConsumed(n); err != nil {
			t.Log(err)
		} else {
			t.FailNow()
		}
	}

	for _, n := range []uint32{uint32(0x100000000 - 10), 10} {
		if enqueue, err := c.onConsumed(n); err != nil {
			t.FailNow()
		} else {
			t.Logf("sequence=%v enqueue=%v", n, enqueue)
		}

		for _, m := range []uint32{uint32(n - 1), 20} {
			if _, err := c.onConsumed(m); err != nil {
				t.Log(err)
			} else {
				t.FailNow()
			}
		}
	}
}
