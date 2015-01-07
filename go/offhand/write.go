package offhand

import (
	"encoding/binary"
	"fmt"
	"math"
)

func writeGeneralPacket(w *alignWriter, packetType uint8) (err error) {
	if err = writePacketHeader(w, false, false, 0, packetType, 0); err != nil {
		return
	}

	return w.Align(8)
}

func writeChannelOpPacket(w *alignWriter, channelIdSize int, ids []ChannelId, packetType uint8) error {
	return writeChannelPacket(w, channelIdSize, ids, protocolFormatChannelOp, packetType)
}

func writeChannelAckPacket(w *alignWriter, channelIdSize int, ids []ChannelId, packetType uint8) error {
	return writeChannelPacket(w, channelIdSize, ids, protocolFormatChannelAck, packetType)
}

func writeChannelPacket(w *alignWriter, channelIdSize int, ids []ChannelId, format, packetType uint8) (err error) {
	if err = writePacketHeader(w, true, len(ids) != 1, format, packetType, 0); err != nil {
		return
	}

	if err = writePacketChannelIds(w, channelIdSize, ids); err != nil {
		return
	}

	return w.Align(8)
}

func writeSequenceOpPacket(w *alignWriter, channelIdSize int, ids []ChannelId, packetType uint8, sequence uint32) error {
	return writeSequencePacket(w, channelIdSize, ids, protocolFormatSequenceOp, packetType, sequence)
}

func writeSequenceAckPacket(w *alignWriter, channelIdSize int, ids []ChannelId, packetType uint8, sequence uint32) error {
	return writeSequencePacket(w, channelIdSize, ids, protocolFormatSequenceAck, packetType, sequence)
}

func writeSequencePacket(w *alignWriter, channelIdSize int, ids []ChannelId, format, packetType uint8, sequence uint32) (err error) {
	if err = writePacketHeader(w, true, len(ids) != 1, format, packetType, 0); err != nil {
		return
	}

	if err = writePacketChannelIds(w, channelIdSize, ids); err != nil {
		return
	}

	if err = w.Align(4); err != nil {
		return
	}

	if err = binary.Write(w, binary.LittleEndian, sequence); err != nil {
		return
	}

	return w.Align(8)
}

func writeMessagePacket(w *alignWriter, channelIdSize int, ids []ChannelId, m Message) (err error) {
	var flags uint8
	var shortLength uint8

	for _, part := range m {
		if len(part) > math.MaxUint16 {
			flags |= protocolMessageFlagLarge
			break
		}
	}

	if len(m) <= math.MaxUint8 {
		shortLength = uint8(len(m))
	} else {
		flags |= protocolMessageFlagLong
	}

	if err = writePacketHeader(w, true, len(ids) != 1, protocolFormatMessage, flags, shortLength); err != nil {
		return
	}

	if err = writePacketChannelIds(w, channelIdSize, ids); err != nil {
		return
	}

	if (flags & protocolMessageFlagLong) != 0 {
		if err = w.Align(2); err != nil {
			return
		}

		if err = binary.Write(w, binary.LittleEndian, uint32(len(m))); err != nil {
			return
		}
	}

	if (flags & protocolMessageFlagLarge) != 0 {
		if err = w.Align(8); err != nil {
			return
		}

		for _, part := range m {
			if err = binary.Write(w, binary.LittleEndian, uint64(len(part))); err != nil {
				return
			}
		}
	} else {
		for _, part := range m {
			if err = binary.Write(w, binary.LittleEndian, uint16(len(part))); err != nil {
				return
			}
		}

		if err = w.Align(8); err != nil {
			return
		}
	}

	for _, part := range m {
		if _, err = w.Write(part); err != nil {
			return
		}

		if err = w.Align(8); err != nil {
			return
		}
	}

	return
}

func writePacketHeader(w *alignWriter, channel, multicast bool, format, x uint8, shortMessageLength uint8) (err error) {
	var header uint8

	if channel {
		header |= protocolHeaderFlagChannel

		if multicast {
			header |= protocolHeaderFlagMulticast
		}

		header |= format << 2
	}

	header |= x << 5

	_, err = w.Write([]byte{header, shortMessageLength})
	return
}

func writePacketChannelIds(w *alignWriter, channelIdSize int, ids []ChannelId) (err error) {
	if len(ids) != 1 {
		if err = w.Pad(2); err != nil {
			return
		}

		if err = binary.Write(w, binary.LittleEndian, uint32(len(ids))); err != nil {
			return
		}
	}

	if channelIdSize > 0 {
		for _, id := range ids {
			buf := []byte(id)
			if len(buf) != channelIdSize {
				panic(fmt.Errorf("bad channel id size %d (expected %d)", len(buf), channelIdSize))
			}

			if _, err = w.Write(buf); err != nil {
				return
			}
		}
	}

	return
}
