__all__ = [
    "CorruptedMessage",
    "Stats",
    "UnexpectedCommand",
    "UnexpectedEOF",
    "UnknownCommand",
    "log",
]

import logging
import struct

log = logging.getLogger("offhand")


class UnexpectedEOF(Exception):

    def __init__(self):
        Exception.__init__(self, "Connection closed unexpectedly")


class UnknownCommand(Exception):

    def __init__(self, command):
        Exception.__init__(self, "Unknown command: %r" % command)


class UnexpectedCommand(Exception):

    def __init__(self, command):
        Exception.__init__(self, "Unexpected command: %r" % command)


class CorruptedMessage(Exception):

    def __init__(self):
        Exception.__init__(self, "Corrupted message")


class Stats(object):
    __slots__ = [
        "connecting",
        "connected",
        "idle",
        "busy",
        "total_engaged",
        "total_canceled",
        "total_rolledback",
        "total_timeouts",
        "total_disconnects",
        "total_errors",
    ]

    def __init__(self, copy=None):
        for key in self.__slots__:
            setattr(self, key, getattr(copy, key) if copy else 0)

    def __nonzero__(self):
        return any(getattr(self, key) for key in self.__slots__)

    def __str__(self):
        return " ".join("%s=%s" % (key, getattr(self, key)) for key in self.__slots__)


def parse_message(data):
    message = []
    offset = 0

    while True:
        remain = len(data) - offset
        if remain == 0:
            break

        if remain < 4:
            raise CorruptedMessage()

        part_size, = struct.unpack("<I", data[offset: offset + 4])
        offset += 4

        if remain < 4 + part_size:
            raise CorruptedMessage()

        message.append(data[offset: offset + part_size])
        offset += part_size

    return message
