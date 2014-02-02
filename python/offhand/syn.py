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
	UnexpectedCommand,
	log,
	parse_message,
)

from .protocol import *

class Reconnect(Exception):

	def __init__(self, timedout=False, eof=False, initial=False):
		self.timedout = timedout
		self.eof = eof
		self.initial = initial

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
		timedout = False

		try:
			sock = self.socket()
			sock.settimeout(self.timeout)
			sock.connect(self.address)
			ok = True
		except socket.error as e:
			if e.errno not in self.soft_connect_errors:
				log.exception("%s: connect", self)
			timedout = (e.errno == errno.ETIMEDOUT)
		except Exception:
			log.exception("%s: connect", self)

		if ok:
			self.sock = sock
		else:
			if sock:
				sock.close()

			raise Reconnect(timedout)

	def send(self, data):
		n = 0

		while n < len(data):
			ok = False
			timedout = False
			eof = False

			try:
				ret = self.sock.send(data[n:])
				if ret == 0:
					log.error("%s: unexpected EOF", self)
					eof = True
				else:
					n += ret
					ok = True
			except socket.error as e:
				if e.errno == errno.EAGAIN:
					continue
				log.error("%s: send: %s", self, e)
				timedout = (e.errno == errno.ETIMEDOUT)
				eof = (e.errno == errno.ECONNRESET)
			except Exception:
				log.exception("%s: send", self)

			if not ok:
				raise Reconnect(timedout, eof)

	def recv(self, size, initial=False):
		data = b""

		while len(data) < size:
			buf = None
			timedout = False
			eof = False

			try:
				buf = self.sock.recv(size - len(data))
			except socket.timeout as e:
				if not initial:
					log.error("%s: recv: %s", self, e)
				timedout = True
			except socket.error as e:
				if e.errno == errno.EAGAIN:
					continue
				log.error("%s: recv: %s", self, e)
				timedout = (e.errno == errno.ETIMEDOUT)
				eof = (e.errno == errno.ECONNRESET)
			except Exception:
				log.exception("%s: recv", self)
			else:
				if not buf:
					if not initial or data:
						log.error("%s: unexpected EOF", self)
					eof = True

			if not buf:
				raise Reconnect(timedout, eof, initial)

			data += buf

		return data

def connect_pull(handler, address, stats, connection_type=Connection):
	with connection_type(address) as conn:
		conn_stat = ConnectionStat(stats)
		occu_stat = OccupationStat(stats)
		delay = False

		while True:
			with conn_stat:
				conn.close()

				if delay:
					time.sleep(1)

				delay = True

				try:
					conn.connect()
					conn_stat.trigger()

					while True:
						with occu_stat:
							command, = conn.recv(1, initial=True)
							occu_stat.trigger()

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
								stats.total_rolledback += 1
								continue
							else:
								raise UnexpectedCommand(command)

							if handler(parse_message(data), start_time):
								conn.send(REPLY_ENGAGED)
								stats.total_engaged += 1
							else:
								conn.send(REPLY_CANCELED)
								stats.total_canceled += 1
				except Reconnect as e:
					if e.timedout or e.eof:
						delay = False

					if e.timedout:
						stats.total_timeouts += 1
					elif e.eof and e.initial:
						stats.total_disconnects += 1
					else:
						stats.total_errors += 1
				except (CorruptedMessage, UnexpectedCommand) as e:
					log.error("%s: %s", conn, e)
					stats.total_errors += 1

class Stat(object):

	def __init__(self, stats):
		self.stats = stats

	def __enter__(self):
		self.update_primary(1)
		self._triggered = False

	def __exit__(self, *exc):
		if self._triggered:
			self.update_secondary(-1)
		else:
			self.update_primary(-1)

	def trigger(self):
		self.update_secondary(1)
		self.update_primary(-1)
		self._triggered = True

class ConnectionStat(Stat):

	def update_primary(self, value):
		self.stats.connecting += value

	def update_secondary(self, value):
		self.stats.connected += value

class OccupationStat(object):

	def update_primary(self, value):
		self.stats.idle += value

	def update_secondary(self, value):
		self.stats.busy += value
