def byte(x):
    return bytes(bytearray([x]))


COMMAND_BEGIN = byte(10)
COMMAND_COMMIT = byte(21)
COMMAND_ROLLBACK = byte(30)
COMMAND_KEEPALIVE = byte(40)

REPLY_RECEIVED = byte(11)
REPLY_ENGAGED = byte(21)
REPLY_CANCELED = byte(22)
REPLY_KEEPALIVE = byte(41)
