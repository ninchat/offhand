GO	:= go
PYTHON	:= python
SHELL	:= bash

default::

check:: go/go.test
	@ rm -rf .sockets
	@ mkdir -p .sockets
	$(PYTHON) python/offhand_test.py

go/go.test: $(wildcard go/*.go)
	cd go && $(GO) test -c

clean::
	rm -f python/*.py[co] go/go.test
	rm -rf python/build .sockets
