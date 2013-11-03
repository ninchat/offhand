__all__ = [
	"CorruptedMessage",
	"Stats",
	"UnexpectedCommand",
	"UnexpectedEOF",
	"UnknownCommand",
]

import operator

class UnexpectedEOF(Exception):

	def __init__(self):
		Exception.__init__(self, "Connection closed unexpectedly")

class UnknownCommand(Exception):

	def __init__(self, command):
		Exception.__init__(self, "Unknown command: %d" % ord(command))

class UnexpectedCommand(Exception):

	def __init__(self, command):
		Exception.__init__(self, "Unexpected command: %d" % ord(command))

class CorruptedMessage(Exception):

	def __init__(self):
		Exception.__init__(self, "Corrupted message")

class Stats(object):

	__slots__ = [
		"begin",
		"commit",
		"rollback",
		"engage",
		"cancel",
		"error",
		"reconnect",
		"disconnect",
	]

	def __init__(self, copy=None):
		for key in self.__slots__:
			setattr(self, key, getattr(copy, key) if copy else 0)

	def __nonzero__(self):
		return any(getattr(self, key) for key in self.__slots__)

	def __str__(self):
		return " ".join("%s=%s" % (key, getattr(self, key)) for key in self.__slots__)

	def __add__(self, x):
		return self.__operate(operator.add, lambda key: getattr(x, key))

	def __sub__(self, x):
		return self.__operate(operator.sub, lambda key: getattr(x, key))

	def __div__(self, x):
		return self.__operate(operator.div, lambda key: x)

	def __operate(self, op, getter):
		r = type(self)(self)
		for key in self.__slots__:
			setattr(r, key, op(getattr(self, key), getter(key)))
		return r
