import asyncore
import os
import socket
import subprocess
import sys
import threading
import time

import offhand

messages = [
	# TestSequence
	["\xf0\x00\x0d"],
	[],
	["", "\xde\xad\xbe\xef\xca\xfe\xba", "", "", "\xbe", ""],

	# TestParallel
	["\xff\xee\xdd"],
	[],
	["", "\xcc\xcc\xcc\xbb\xbb\xbb\xaa", "", "", "\xaa", ""],
]

class Puller(offhand.AsynConnectPuller):

	class Node(offhand.AsynConnectPuller.Node):
		socket_family = socket.AF_UNIX

	def handle_pull(self, node, message):
		print self, node, "message", message
		messages.remove(message)

def test(done):
	os.chdir(".test/sockets")

	with Puller() as p1, Puller() as p2:
		pullers = p1, p2

		for i in xrange(3):
			for p in pullers:
				p.connect(str(i))

		timeout   = 0.1
		last_time = time.time()

		while True:
			t = timeout - (time.time() - last_time)
			if t <= 0:
				for p in pullers:
					p.reconnect()

				last_time = time.time()
				t = timeout

			if asyncore.socket_map:
				asyncore.loop(timeout=t, use_poll=True, count=1)
			elif done[0]:
				return
			else:
				time.sleep(t)

def main():
	done = [False]

	t = threading.Thread(target=test, args=(done,))
	t.daemon = True

	p = subprocess.Popen(["./offhand.test", "-test.v=true"])
	try:
		t.start()
	finally:
		status = p.wait()
		if status == 0:
			print "go test exited"
		else:
			print >>sys.stderr, "go test exited with %r" % status

	done[0] = True
	t.join(2)
	if t.is_alive():
		print >>sys.stderr, "python test exit timeout"
	else:
		print "python test exited"

		if messages:
			print >>sys.stderr, "remaining messages:"
			for m in messages:
				print >>sys.stderr, "  %r" % m

if __name__ == "__main__":
	try:
		main()
	except KeyboardInterrupt:
		print
