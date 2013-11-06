from __future__ import absolute_import

__all__ = [
	"Connection",
	"connect_pull",
]

import errno
import socket
import struct
import time

from . import (
	CorruptedMessage,
	Stats,
	UnexpectedCommand,
	log,
	parse_message,
)

from .protocol import *

class Reconnect(Exception):
	pass

class Connection(object):
	socket_family = socket.AF_INET
	socket_type = socket.SOCK_STREAM

	soft_connect_errors = (
		errno.ECONNREFUSED,
		errno.ECONNRESET,
		errno.ENOENT,
		errno.ETIMEDOUT,
	)

	@classmethod
	def socket(cls):
		return socket.socket(cls.socket_family, cls.socket_type)

	def __init__(self, address):
		self.address = address
		self.sock = None

	def __str__(self):
		if isinstance(self.address, tuple):
			return ":".join(str(x) for x in self.address)
		else:
			return self.address

	def __enter__(self):
		return self

	def __exit__(self, *exc):
		self.close()

	def close(self):
		if self.sock:
			try:
				self.sock.close()
			finally:
				self.sock = None

	def connect(self):
		assert self.sock is None

		sock = None
		ok = False

		try:
			sock = self.socket()
			sock.connect(self.address)
			ok = True
		except socket.error as e:
			if e.errno not in self.soft_connect_errors:
				log.exception("%s: connect", self)
		except Exception:
			log.exception("%s: connect", self)

		if ok:
			self.sock = sock
		else:
			if sock:
				sock.close()

			raise Reconnect()

	def send(self, data):
		n = 0

		while n < len(data):
			ok = False

			try:
				n += self.sock.send(data[n:])
				ok = True
			except socket.error as e:
				if e.errno == errno.EAGAIN:
					continue
				log.exception("%s: send", self)
			except Exception:
				log.exception("%s: send", self)

			if not ok:
				raise Reconnect()

	def recv(self, size, eof_ok=False):
		data = b""

		while len(data) < size:
			buf = None

			try:
				buf = self.sock.recv(size - len(data))
			except socket.error as e:
				if e.errno == errno.EAGAIN:
					continue
				log.exception("%s: recv", self)
			except Exception:
				log.exception("%s: recv", self)

			if not buf:
				if not eof_ok or data:
					log.error("%s: unexpected EOF", self)

				raise Reconnect()

			data += buf

		return data

def connect_pull(handler, address, connection_type=Connection):
	with connection_type(address) as conn:
		delay = False

		while True:
			conn.close()

			if delay:
				time.sleep(1)

			delay = True

			try:
				conn.connect()

				command, = conn.recv(1, eof_ok=True)
				if command != COMMAND_BEGIN:
					raise UnexpectedCommand(command)

				size, = struct.unpack(b"<I", conn.recv(4))
				data = conn.recv(size)

				conn.send(REPLY_RECEIVED)

				command, = conn.recv(1)
				if command == COMMAND_OLDCOMMIT:
					start_time = time.time()
				elif command == COMMAND_COMMIT:
					latency, = struct.unpack(b"<I", conn.recv(4))
					start_time = time.time() - latency / 1000000.0
				elif command == COMMAND_ROLLBACK:
					continue
				else:
					raise UnexpectedCommand(command)

				engaged = handler(parse_message(data), start_time)

				conn.send(REPLY_ENGAGED if engaged else REPLY_CANCELED)
			except Reconnect:
				pass
			except (CorruptedMessage, UnexpectedCommand) as e:
				log.error("%s: %s", conn, e)
