import struct

def bits(data):
	s = ""
	for c in data:
		byte, = struct.unpack("<B", c)
		for bit in xrange(8):
			if (1 << bit) & byte:
				s += "1"
			else:
				s += "0"
		s += " "
	print s

bits(struct.pack("<BBB", 2, 0, 0))
bits(struct.pack("<H", 2047 | 0x8000))
bits(struct.pack("<H", 0))
