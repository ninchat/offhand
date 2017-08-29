from __future__ import absolute_import

__all__ = [
    "connect_pull",
]

import gevent
import gevent.event
import gevent.pool

from . import syn


class Commit(syn.Commit):

    def __init__(self):
        super().__init__()
        self._event = gevent.event.Event()

    def engage(self):
        if self.closed:
            assert self.engaged

        self.engaged = True
        self.closed = True
        self._event.set()

    def cancel(self):
        assert not self.closed

        self.closed = True
        self._event.set()

    def close(self):
        if not self.closed:
            self.cancel()

    def wait(self):
        self._event.wait()
        return self.engaged


def connect_pull(handler, address, group=None, *args, **kwargs):
    if not group:
        group = gevent.pool.Group()

    def commit_handler(message, start_time, commit):
        with commit:
            handler(message, start_time, commit)

    def result_handler(message, start_time):
        commit = Commit()
        group.spawn(commit_handler, message, start_time, commit)
        return commit._wait()

    try:
        syn.connect_pull(result_handler, address, *args, **kwargs)
    except gevent.GreenletExit:
        pass

    return group
