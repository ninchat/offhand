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
PIPESED		:= | sed -r "s,^\t([a-z_]+\.go:),relink/\1,"

export GOPATH
export GOMAXPROCS

all:: fmt-vet check

fmt-vet::
	$(GOFMT) -d -s ./relink ./relish
	$(GO) vet ./relink ./relish

check::
#	$(GO) help testflag
	$(GO) test -v $(TESTFLAGS) ./relink ./relish $(PIPESED)

../relish::
	$(GO) build -o $@ ./relish
