from __future__ import absolute_import

__all__ = [
	"ConnectPuller",
]

import asyncore
import errno
import socket
import struct
import time

from . import (
	CorruptedMessage,
	Stats,
	UnexpectedCommand,
	UnexpectedEOF,
	UnknownCommand,
)

from .protocol import *

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

class ValueBuffer(object):

	valuelen = 4
	valuefmt = "<I"

	def __init__(self):
		self.buf = ""

	def receive(self, s):
		b = s.recv(self.valuelen - len(self.buf))
		if not b:
			raise UnexpectedEOF()

		self.buf += b

		if len(self.buf) < self.valuelen:
			return None

		value, = struct.unpack(self.valuefmt, self.buf)
		return value

class Commit(object):

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

class ConnectNode(asyncore.dispatcher):

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
		except socket.gaierror:
			self.handle_close()
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
			b = self.recv(1)
			if not b:
				if self.message is None:
					self.handle_close()
					return
				else:
					raise UnexpectedEOF()

			self.command = b[0]

			if self.command == COMMAND_BEGIN:
				assert self.buffer is None

				if self.message is not None:
					raise UnexpectedCommand(self.command)

				self.stats.begin += 1

				self.buffer = Buffer()

			elif self.command == COMMAND_OLDCOMMIT:
				assert self.buffer is None

				if self.message is None:
					raise UnexpectedCommand(self.command)

				self.stats.commit += 1

				self.commit = Commit(self)
				self.puller.handle_pull(self, self.message, time.time(), self.commit)

				if self.commit:
					return

			elif self.command == COMMAND_COMMIT:
				if self.message is None:
					raise UnexpectedCommand(self.command)

				if self.buffer is None:
					self.buffer = ValueBuffer()

				latency = self.buffer.receive(self)
				if latency is None:
					return

				start_time = time.time() - latency / 1000000.0

				self.stats.commit += 1

				self.commit = Commit(self)
				self.puller.handle_pull(self, self.message, start_time, self.commit)

				if self.commit:
					return

			elif self.command == COMMAND_ROLLBACK:
				assert self.buffer is None

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
		except socket.gaierror:
			self.handle_close()
		except socket.error as e:
			if e.errno in self.soft_errors:
				self.handle_close()
			else:
				raise

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

class ConnectPuller(object):

	Node = ConnectNode

	def __init__(self):
		self.__reset()

	def __reset(self):
		self.nodes = {}
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
		return sum((n.stats for n in self.nodes.itervalues()), Stats())

	def connect(self, address, *args, **kwargs):
		assert address not in self.nodes
		node = self.Node(self, address, *args, **kwargs)
		self.nodes[address] = node
		node.reconnect()

	def disconnect(self, address):
		node = self.nodes.pop(address)
		try:
			self.disconnected.remove(node)
		except KeyError:
			node.close()

	def reconnect(self):
		reconnecting = self.disconnected
		self.disconnected = set()

		for node in reconnecting:
			node.reconnect()

	def close(self):
		for node in self.nodes.itervalues():
			if node not in self.disconnected:
				node.close()

		self.__reset()

	def handle_pull(self, node, message, start_time, commit):
		commit.cancel()
		assert not "handled pull event"

	def handle_disconnect(self, node):
		self.disconnected.add(node)