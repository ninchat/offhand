export GOROOT	:= $(HOME)/nin/src/code.google.com/p/go
GO		:= $(GOROOT)/bin/go

test: $(wildcard *.go)
	$(GO) test

clean::
	rm -f test
