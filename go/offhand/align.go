package offhand

import (
	"bufio"
	"io"
)

// alignSize rounds up to the next 64-bit word.
func alignSize(n uint) uint {
	return (n + 7) &^ 7
}

// alignReader can read padding.
type alignReader struct {
	r io.Reader
	n uint
}

func newAlignReader(r io.Reader) *alignReader {
	return &alignReader{
		r: r,
	}
}

func (ar *alignReader) Read(b []byte) (n int, err error) {
	n, err = ar.r.Read(b)
	ar.n += uint(n)
	return
}

func (ar *alignReader) Align(alignment uint) (err error) {
	offset := ar.n & (alignment - 1)
	if offset > 0 {
		ar.Pad(alignment - offset)
	}
	return
}

func (ar *alignReader) Pad(size uint) (err error) {
	n, err := io.ReadFull(ar.r, make([]byte, size))
	ar.n += uint(n)
	return
}

// alignWriter can write padding.
type alignWriter struct {
	bf *bufio.Writer
	n  uint
}

func newAlignWriter(bf *bufio.Writer) *alignWriter {
	return &alignWriter{
		bf: bf,
	}
}

func (aw *alignWriter) Write(b []byte) (n int, err error) {
	n, err = aw.bf.Write(b)
	aw.n += uint(n)
	return
}

func (aw *alignWriter) Align(alignment uint) (err error) {
	offset := aw.n & (alignment - 1)
	if offset > 0 {
		aw.Pad(alignment - offset)
	}
	return
}

func (aw *alignWriter) Pad(size uint) (err error) {
	n, err := aw.bf.Write(make([]byte, size))
	aw.n += uint(n)
	return
}

func (aw *alignWriter) Flush() error {
	return aw.bf.Flush()
}
