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

	def __init__(self, notbad):
		self.notbad = notbad

class Connection(object):
	socket_family = socket.AF_INET
	socket_type = socket.SOCK_STREAM
	timeout = 34

	soft_connect_errors = (
		errno.ECONNREFUSED,
		errno.ECONNRESET,
		errno.ENOENT,
		errno.ETIMEDOUT,
	)

	@classmethod
	def socket(cls):
		sock = socket.socket(cls.socket_family, cls.socket_type)
		sock.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)
		sock.setsockopt(socket.SOL_TCP, socket.TCP_KEEPCNT, 1)
		sock.setsockopt(socket.SOL_TCP, socket.TCP_KEEPIDLE, 19)
		sock.setsockopt(socket.SOL_TCP, socket.TCP_NODELAY, 1)
		return sock

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
		notbad = False

		try:
			sock = self.socket()
			sock.settimeout(self.timeout)
			sock.connect(self.address)
			ok = True
		except socket.error as e:
			if e.errno not in self.soft_connect_errors:
				log.exception("%s: connect", self)
			notbad = (e.errno == errno.ETIMEDOUT)
		except Exception:
			log.exception("%s: connect", self)

		if ok:
			self.sock = sock
		else:
			if sock:
				sock.close()

			raise Reconnect(notbad)

	def send(self, data):
		n = 0

		while n < len(data):
			ok = False
			notbad = False

			try:
				ret = self.sock.send(data[n:])
				if ret == 0:
					log.error("%s: unexpected EOF", self)
					notbad = True
				else:
					n += ret
					ok = True
			except socket.error as e:
				if e.errno == errno.EAGAIN:
					continue
				log.error("%s: send: %s", self, e)
				notbad = e.errno in (errno.ECONNRESET, errno.ETIMEDOUT)
			except Exception:
				log.exception("%s: send", self)

			if not ok:
				raise Reconnect(notbad)

	def recv(self, size, initial=False):
		data = b""

		while len(data) < size:
			buf = None
			notbad = False

			try:
				buf = self.sock.recv(size - len(data))
			except socket.timeout as e:
				if not initial:
					log.error("%s: recv: %s", self, e)
				notbad = True
			except socket.error as e:
				if e.errno == errno.EAGAIN:
					continue
				log.error("%s: recv: %s", self, e)
				notbad = e.errno in (errno.ECONNRESET, errno.ETIMEDOUT)
			except Exception:
				log.exception("%s: recv", self)
			else:
				if not buf:
					if not initial or data:
						log.error("%s: unexpected EOF", self)
					notbad = True

			if not buf:
				raise Reconnect(notbad)

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

				while True:
					command, = conn.recv(1, initial=True)
					if command == COMMAND_KEEPALIVE:
						conn.send(REPLY_KEEPALIVE)
						continue
					elif command != COMMAND_BEGIN:
						raise UnexpectedCommand(command)

					size, = struct.unpack(b"<I", conn.recv(4))
					data = conn.recv(size)

					conn.send(REPLY_RECEIVED)

					command, = conn.recv(1)
					if command == COMMAND_COMMIT:
						latency, = struct.unpack(b"<I", conn.recv(4))
						start_time = time.time() - latency / 1000000.0
					elif command == COMMAND_ROLLBACK:
						continue
					else:
						raise UnexpectedCommand(command)

					engaged = handler(parse_message(data), start_time)

					conn.send(REPLY_ENGAGED if engaged else REPLY_CANCELED)
			except Reconnect as e:
				if e.notbad:
					delay = False
			except (CorruptedMessage, UnexpectedCommand) as e:
				log.error("%s: %s", conn, e)
