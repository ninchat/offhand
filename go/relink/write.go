package relink

import (
	"encoding/binary"
	"math"
)

func writeGeneralPacket(w *alignWriter, packetType uint8) (err error) {
	if err = writePacketHeader(w, false, false, 0, packetType, 0); err != nil {
		return
	}

	return w.Align(8)
}

func writeChannelOpPacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, packetType uint8) error {
	return writeChannelPacket(w, traits, ids, protocolFormatChannelOp, packetType)
}

func writeChannelAckPacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, packetType uint8) error {
	return writeChannelPacket(w, traits, ids, protocolFormatChannelAck, packetType)
}

func writeChannelPacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, format, packetType uint8) (err error) {
	if err = writePacketHeader(w, true, len(ids) != 1, format, packetType, 0); err != nil {
		return
	}

	if err = writePacketChannelIds(w, traits, ids); err != nil {
		return
	}

	return w.Align(8)
}

func writeSequenceOpPacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, packetType uint8, sequence uint32) error {
	return writeSequencePacket(w, traits, ids, protocolFormatSequenceOp, packetType, sequence)
}

func writeSequenceAckPacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, packetType uint8, sequence uint32) error {
	return writeSequencePacket(w, traits, ids, protocolFormatSequenceAck, packetType, sequence)
}

func writeSequencePacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, format, packetType uint8, sequence uint32) (err error) {
	if err = writePacketHeader(w, true, len(ids) != 1, format, packetType, 0); err != nil {
		return
	}

	if err = writePacketChannelIds(w, traits, ids); err != nil {
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

func writeMessagePacket(w *alignWriter, traits ChannelTraits, ids []ChannelId, m Message) (err error) {
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

	if err = writePacketChannelIds(w, traits, ids); err != nil {
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

func writePacketChannelIds(w *alignWriter, traits ChannelTraits, ids []ChannelId) (err error) {
	if len(ids) != 1 {
		if err = w.Pad(2); err != nil {
			return
		}

		if err = binary.Write(w, binary.LittleEndian, uint32(len(ids))); err != nil {
			return
		}
	}

	if traits != nil {
		for _, id := range ids {
			if err = binary.Write(w, binary.LittleEndian, id); err != nil {
				return
			}
		}
	}

	return
}
