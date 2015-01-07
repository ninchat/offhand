-include local.mk

ifeq ($(GOROOT),)
GO		:= go
else
GO		:= $(GOROOT)/bin/go
endif

GOMAXPROCS	:= $(shell getconf _NPROCESSORS_ONLN || true)
GOPATH		:= $(shell pwd)
GOFMT		:= $(GO)fmt
TESTFLAGS	:= 
PIPESED		:= | sed -r "s,^\t([a-z_]+\.go:),offhand/\1,"

export GOPATH
export GOMAXPROCS

all:: fmt-vet check

fmt-vet::
	$(GOFMT) -d -s ./offhand ./shell
	$(GO) vet ./offhand ./shell

check::
#	$(GO) help testflag
	$(GO) test -v $(TESTFLAGS) ./offhand ./shell $(PIPESED)

../shell::
	$(GO) build -o $@ ./shell
