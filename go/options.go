package offhand

type Options struct {
	IdSize       int
	Transactions bool
}

func (o Options) marshal() (b byte) {
	b = byte(o.IdSize & 0x7f)
	if o.Transactions {
		b |= byte(0x80)
	}
	return
}
