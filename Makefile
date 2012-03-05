GO	:= go
PYTHON	:= python
SHELL	:= bash

default::

check:: offhand.test
	@ rm -rf .test/sockets
	@ mkdir .test/sockets
	$(PYTHON) offhand_test.py

offhand.test: $(wildcard *.go) .test/src/offhand
	@ echo $(GO) test -c offhand
	@ GOPATH=$(PWD)/.test $(GO) test -c offhand 2>&1 | sed 's,[^:[:space:]]*\.test/src/offhand/,,g'; exit $${PIPESTATUS[0]}

.test/src/offhand:
	@ mkdir -p .test/src
	@ ln -s ../.. .test/src/offhand

clean::
	rm -f *.py[co] offhand.test
	rm -rf build .test
