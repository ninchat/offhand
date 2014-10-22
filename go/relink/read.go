package relink

import (
	"encoding/binary"
	"fmt"
	"io"
)

func readPacketHeader(r io.Reader) (channel, multicast bool, format, x uint8, err error) {
	header := make([]byte, 1)

	if _, err = io.ReadFull(r, header); err != nil {
		return
	}

	channel = (header[0] & protocolHeaderFlagChannel) != 0
	multicast = (header[0] & protocolHeaderFlagMulticast) != 0
	format = (header[0] >> 2) & 7
	x = header[0] >> 5
	return
}

func readPacketShortMessageLength(r io.Reader) (shortMessageLength uint8, err error) {
	header := make([]byte, 1)

	if _, err = io.ReadFull(r, header); err != nil {
		return
	}

	shortMessageLength = header[0]
	return
}

func readUnicastPacketChannelIds(r io.Reader, channelIdSize int) (ids []ChannelId, err error) {
	ids = make([]ChannelId, 1)

	if channelIdSize > 0 {
		buf := make([]byte, channelIdSize)

		if _, err = io.ReadFull(r, buf); err != nil {
			return
		}

		ids[0] = ChannelId(buf)
	}

	return
}

func readMulticastPacketChannelIds(r *alignReader, channelIdSize int, maxCount int) (ids []ChannelId, err error) {
	if err = r.Pad(2); err != nil {
		return
	}

	var wireCount uint32

	if err = binary.Read(r, binary.LittleEndian, &wireCount); err != nil {
		return
	}

	if wireCount > uint32(maxCount) {
		err = fmt.Errorf("multicast count %d exceeds limit %d", wireCount, maxCount)
		return
	}

	count := int(wireCount)
	ids = make([]ChannelId, count)

	if channelIdSize > 0 {
		buf := make([]byte, channelIdSize*count)

		if _, err = io.ReadFull(r, buf); err != nil {
			return
		}

		for i := 0; i < count; i++ {
			ids[i] = ChannelId(buf[channelIdSize*i : channelIdSize*(i+1)])
		}
	}

	return
}

func readLongMessagePacketLength(r io.Reader) (length uint32, err error) {
	err = binary.Read(r, binary.LittleEndian, &length)
	return
}

func readSmallMessagePacketPayload(r *alignReader, parts Message, maxPartSize int, maxTotalSize int64) (err error) {
	sizes := make([]uint16, len(parts))

	var totalSize uint64

	for i := range sizes {
		var size uint16

		if err = binary.Read(r, binary.LittleEndian, &size); err != nil {
			return
		}

		if int(size) > maxPartSize {
			err = fmt.Errorf("message part size %d exceeds limit %d", size, maxPartSize)
			return
		}

		totalSize += uint64(size)
		if totalSize > uint64(maxTotalSize) {
			err = fmt.Errorf("message total size %d exceeds limit %d", totalSize, maxTotalSize)
			return
		}

		sizes[i] = size
	}

	if err = r.Align(8); err != nil {
		return
	}

	for i, size := range sizes {
		if size > 0 {
			part := make([]byte, alignSize(uint(size)))

			if _, err = io.ReadFull(r, part); err != nil {
				return
			}

			parts[i] = part[:size]
		}
	}

	return
}

func readLargeMessagePacketPayload(r *alignReader, parts Message, maxPartSize int, maxTotalSize int64) (err error) {
	if err = r.Align(8); err != nil {
		return
	}

	sizes := make([]uint64, len(parts))

	var totalSize uint64

	for i := range sizes {
		var size uint64

		if err = binary.Read(r, binary.LittleEndian, &size); err != nil {
			return
		}

		if size > uint64(maxPartSize) {
			err = fmt.Errorf("message part size %d exceeds limit %d", size, maxPartSize)
			return
		}

		totalSize += size
		if totalSize > uint64(maxTotalSize) {
			err = fmt.Errorf("message total size %d exceeds limit %d", totalSize, maxTotalSize)
			return
		}

		sizes[i] = size
	}

	for i, size := range sizes {
		if size > 0 {
			part := make([]byte, alignSize(uint(size)))

			if _, err = io.ReadFull(r, part); err != nil {
				return
			}

			parts[i] = part[:size]
		}
	}

	return
}

func readPacketSequence(r *alignReader) (sequence uint32, err error) {
	if err = r.Align(4); err != nil {
		return
	}

	err = binary.Read(r, binary.LittleEndian, &sequence)
	return
}
