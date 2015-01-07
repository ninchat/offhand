package offhand

// Stats of endpoint and link activity.  The fields are incremented using
// atomic operations.
type Stats struct {
	SentPackets      uint64
	SentMessages     uint64
	ReceivedPackets  uint64
	ReceivedMessages uint64
}
