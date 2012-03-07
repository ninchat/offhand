import asyncore
import errno
import socket
import struct
import time

__all__ = ["AsynConnectPuller"]

COMMAND_BEGIN    = chr(1)
COMMAND_COMMIT   = chr(2)
COMMAND_ROLLBACK = chr(3)

REPLY_BEGIN      = chr(1)
REPLY_COMMIT     = chr(2)

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

class Buffer(object):

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

class AsynConnectNode(asyncore.dispatcher):

	socket_family = socket.AF_INET
	socket_type   = socket.SOCK_STREAM

	soft_errors   = errno.ECONNREFUSED, errno.ECONNRESET, errno.ENOENT

	def __init__(self, puller, address, *args, **kwargs):
		asyncore.dispatcher.__init__(self, *args, **kwargs)

		self.puller  = puller
		self.address = address

		self.reset()

	def __str__(self):
		if isinstance(self.address, tuple) and len(self.address) == 2:
			addr = "%s:%s" % self.address
		else:
			addr = self.address

		return "<Node %s>" % addr

	def reconnect(self):
		self.create_socket(self.socket_family, self.socket_type)

		try:
			self.connect(self.address)
		except socket.error as e:
			if e.errno in self.soft_errors:
				self.handle_close()
			else:
				self.handle_error()
		except:
			self.handle_error()

	def reset(self):
		self.command = None
		self.buffer  = None
		self.message = None
		self.reply   = None

	def readable(self):
		return (self.command is None or self.buffer is not None)

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

				self.buffer = Buffer()

			elif self.command == COMMAND_COMMIT:
				if self.message is None:
					raise UnexpectedCommand(self.command)

				self.reply = REPLY_COMMIT

			elif self.command == COMMAND_ROLLBACK:
				if self.message is None:
					raise UnexpectedCommand(self.command)

				self.reset()
			else:
				raise InvalidCommand(self.command)

		if self.buffer is not None:
			assert self.command == COMMAND_BEGIN

			m = self.buffer.receive(self)
			if m is not None:
				self.command = None
				self.buffer  = None
				self.message = m
				self.reply   = REPLY_BEGIN

	def __write(self):
		if self.send(self.reply):
			if self.reply == REPLY_COMMIT:
				self.puller.handle_pull(self, self.message)
				self.reset()
			else:
				self.reply = None

	def handle_close(self):
		self.reset()
		self.close()

		self.puller.handle_disconnect(self)

	def handle_read_event(self):
		try:
			asyncore.dispatcher.handle_read_event(self)
		except socket.error as e:
			if e.errno not in self.soft_errors:
				raise

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

	def handle_pull(self, node, message):
		assert not "handled pull event"

	def handle_disconnect(self, node):
		self.disconnected.add(node)
