package relink

// The highest Relink protocol version supported by this package.
const ProtocolVersion = 0

const (
	protocolHandshakeFlagConnectorTransactions = 1 << iota
	protocolHandshakeFlagListenerTransactions  = 1 << iota
	protocolHandshakeFlagRequireOldLink        = 1 << iota

	protocolHandshakeTransactionFlagMask = protocolHandshakeFlagConnectorTransactions | protocolHandshakeFlagListenerTransactions
)

const (
	protocolHeaderFlagChannel   = 1 << iota
	protocolHeaderFlagMulticast = 1 << iota
)

const (
	protocolFormatFlagReceiveChannel = 1 << iota
)

const (
	protocolFormatChannelOp = iota
	protocolFormatChannelAck
	protocolFormatSequenceOp
	protocolFormatSequenceAck
	protocolFormatMessage
)

const (
	protocolGeneralNop = iota
	protocolGeneralPing
	protocolGeneralPong
	protocolGeneralResume
	protocolGeneralShutdown
)

const (
	protocolChannelOpCommit = iota
	protocolChannelOpRollback
	protocolChannelOpClose
)

const (
	protocolChannelAckReceived = iota
	protocolChannelAckConsumed
	protocolChannelAckCommitted
	protocolChannelAckUncommitted
	protocolChannelAckClosed = iota
)

const (
	protocolSequenceOpCommit = iota
	protocolSequenceOpRollback
)

const (
	protocolSequenceAckReceived = iota
	protocolSequenceAckConsumed
	protocolSequenceAckCommitted
	protocolSequenceAckUncommitted
)

const (
	protocolMessageFlagLong  = 1 << iota
	protocolMessageFlagLarge = 1 << iota
)
