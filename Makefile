GO	:= go
PYTHON	:= python
ANT	:= ant
SHELL	:= bash

default::

check:: go/go.test
	@ rm -rf .sockets
	@ mkdir -p .sockets
	$(PYTHON) python/asyn_test.py

go/go.test: $(wildcard go/*.go)
	cd go && $(GO) test -c

clean::
	rm -f python/*.py[co] go/go.test
	rm -rf python/build .sockets
	- cd java && $(ANT) clean
