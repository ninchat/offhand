import asyncore
import errno
import operator
import socket
import struct
import time

__all__ = ["AsynConnectPuller", "Stats"]

COMMAND_BEGIN    = chr(10)
COMMAND_COMMIT   = chr(20)
COMMAND_ROLLBACK = chr(30)

REPLY_RECEIVED   = chr(11)
REPLY_ENGAGED    = chr(21)
REPLY_CANCELED   = chr(22)

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

class AsynBuffer(object):

	sizelen = 4
	sizefmt = "<I"

	def __init__(self):
		self.sizebuf = ""
		self.size    = None
		self.data    = ""

	def receive(self, s):
		r = self.__receive(s)

		if r == 0:
			raise UnexpectedEOF()

		if r == 2:
			return self.__message()

		return None

	def __receive(self, s):
		r = 0

		if len(self.sizebuf) < self.sizelen:
			b = s.recv(self.sizelen - len(self.sizebuf))
			if not b:
				return r

			self.sizebuf += b
			r = 1

			if len(self.sizebuf) < self.sizelen:
				return r

			self.size, = struct.unpack(self.sizefmt, self.sizebuf)

		if len(self.data) < self.size:
			b = s.recv(self.size - len(self.data))
			if not b:
				return r

			self.data += b
			r = 1

			if len(self.data) < self.size:
				return r

		return 2

	def __message(self):
		message = []
		offset  = 0

		while True:
			remain = self.size - offset
			if remain == 0:
				break

			if remain < self.sizelen:
				raise CorruptedMessage()

			part_size, = struct.unpack(self.sizefmt, self.data[offset:offset+self.sizelen])
			offset += self.sizelen

			if remain < self.sizelen + part_size:
				raise CorruptedMessage()

			message.append(self.data[offset:offset+part_size])
			offset += part_size

		return message

class AsynCommit(object):

	def __init__(self, node):
		self.engage  = lambda: node.engage_commit(self)
		self.cancel  = lambda: node.cancel_commit(self)

		self.closed  = False
		self.engaged = False

	def __enter__(self):
		return self

	def __exit__(self, *exc):
		self.close()

	def close(self):
		if not self.closed:
			self.cancel()

class AsynConnectNode(asyncore.dispatcher):

	socket_family = socket.AF_INET
	socket_type   = socket.SOCK_STREAM

	soft_errors = errno.ECONNREFUSED, errno.ECONNRESET, errno.ENOENT, errno.ETIMEDOUT

	def __init__(self, puller, address, *args, **kwargs):
		asyncore.dispatcher.__init__(self, *args, **kwargs)

		self.puller  = puller
		self.address = address
		self.stats   = Stats()

		self.reset()

	def __str__(self):
		if isinstance(self.address, tuple) and len(self.address) == 2:
			addr = "%s:%s" % self.address
		else:
			addr = self.address

		return "<Node %s>" % addr

	def reconnect(self):
		self.stats.reconnect += 1

		self.create_socket(self.socket_family, self.socket_type)

		try:
			self.connect(self.address)
		except socket.error as e:
			if e.errno in self.soft_errors:
				self.handle_close()
			else:
				self.handle_error()
		except socket.gaierror:
			self.handle_close()
		except:
			self.handle_error()

	def reset(self):
		self.command = None
		self.buffer  = None
		self.message = None
		self.commit  = None
		self.reply   = None

	def readable(self):
		return self.command is None or self.buffer is not None

	def writable(self):
		return self.reply is not None

	def handle_read(self):
		self.__handle(self.__read)

	def handle_write(self):
		self.__handle(self.__write)

	def __handle(self, func):
		try:
			func()
		except socket.error as e:
			if e.errno != errno.EAGAIN:
				self.handle_error()
		except:
			self.handle_error()

	def __read(self):
		assert self.commit is None

		if self.command is None:
			assert self.buffer is None

			b = self.recv(1)
			if not b:
				if self.message is None:
					self.handle_close()
					return
				else:
					raise UnexpectedEOF()

			self.command = b[0]

			if self.command == COMMAND_BEGIN:
				if self.message is not None:
					raise UnexpectedCommand(self.command)

				self.stats.begin += 1

				self.buffer = AsynBuffer()

			elif self.command == COMMAND_COMMIT:
				if self.message is None:
					raise UnexpectedCommand(self.command)

				self.stats.commit += 1

				self.commit = AsynCommit(self)
				self.puller.handle_pull(self, self.message, self.commit)

				if self.commit:
					return

			elif self.command == COMMAND_ROLLBACK:
				if self.message is None:
					raise UnexpectedCommand(self.command)

				self.stats.rollback += 1

				self.reset()
			else:
				raise UnknownCommand(self.command)

		if self.buffer is not None:
			assert self.command == COMMAND_BEGIN

			m = self.buffer.receive(self)
			if m is not None:
				self.command = None
				self.buffer  = None
				self.message = m
				self.reply   = REPLY_RECEIVED

	def __write(self):
		if not self.send(self.reply):
			raise UnexpectedEOF()

		if self.reply == REPLY_RECEIVED:
			self.reply = None
		elif self.reply == REPLY_CANCELED:
			self.reset()
		else:
			assert False

	def handle_close(self):
		self.stats.disconnect += 1

		self.reset()
		self.close()

		self.puller.handle_disconnect(self)

	def handle_error(self):
		self.stats.error += 1

		asyncore.dispatcher.handle_error(self)

	def handle_read_event(self):
		try:
			asyncore.dispatcher.handle_read_event(self)
		except socket.error as e:
			if e.errno in self.soft_errors:
				self.handle_close()
			else:
				raise
		except socket.gaierror:
			self.handle_close()

	def engage_commit(self, commit):
		if commit.closed:
			if commit.engaged:
				return
			else:
				raise Exception("Trying to engage canceled commit")

		assert commit is self.commit

		commit.closed = True

		while True:
			# TODO: don't busy-loop
			try:
				if self.send(REPLY_ENGAGED):
					break
				else:
					raise UnexpectedEOF()
			except socket.error as e:
				if e.errno != errno.EAGAIN:
					self.handle_close()
					raise
			except:
				self.handle_close()
				raise

		commit.engaged = True

		self.reset()

		self.stats.engage += 1

	def cancel_commit(self, commit):
		assert commit is self.commit

		commit.closed = True

		self.commit = None
		self.reply  = REPLY_CANCELED

		self.stats.cancel += 1

class AsynConnectPuller(object):

	Node = AsynConnectNode

	def __init__(self):
		self.__reset()

	def __reset(self):
		self.nodes        = []
		self.disconnected = set()

	def __str__(self):
		dis = len(self.disconnected)
		con = len(self.nodes) - dis

		return "<Puller %x con %u dis %u>" % (id(self), con, dis)

	def __enter__(self):
		return self

	def __exit__(self, *exc):
		self.close()

	@property
	def stats(self):
		return sum((n.stats for n in self.nodes), Stats())

	def connect(self, *args, **kwargs):
		node = self.Node(self, *args, **kwargs)
		self.nodes.append(node)
		node.reconnect()

	def reconnect(self):
		reconnecting = self.disconnected
		self.disconnected = set()

		for node in reconnecting:
			node.reconnect()

	def close(self):
		for node in self.nodes:
			if node not in self.disconnected:
				node.close()

		self.__reset()

	def handle_pull(self, node, message, commit):
		commit.cancel()
		assert not "handled pull event"

	def handle_disconnect(self, node):
		self.disconnected.add(node)
