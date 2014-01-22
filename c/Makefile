CC		:= gcc-4.6
CPPFLAGS	+= -I. -Iccan
CFLAGS		+= -std=c99 -g -Wall -Wextra -Wno-unused-parameter

SOURCES		:= test.c \
		   $(wildcard offhand/*.c) \
		   $(wildcard ccan/ccan/hash/*.c) \
		   $(wildcard ccan/ccan/htable/*.c) \
		   $(wildcard ccan/ccan/objset/*.c) \
		   $(wildcard ccan/ccan/tcon/*.c)

test: $(SOURCES) $(wildcard *.h) $(wildcard offhand/*.h)
	$(CC) $(CPPFLAGS) $(CFLAGS) $(LDFLAGS) -o $@ $(SOURCES) $(LIBS)

check:: test
	./test

clean::
	rm -f test
