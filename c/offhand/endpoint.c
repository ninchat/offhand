#include "internal.h"

#include <assert.h>
#include <errno.h>
#include <stddef.h>
#include <stdlib.h>

#include <sys/socket.h>
#include <sys/types.h>
#include <netdb.h>
#include <unistd.h>

#include <ccan/objset/objset.h>

#include "system.h"

struct socket_set {
	OBJSET_MEMBERS(oh_socket);
};

struct oh_endpoint {
	const struct oh_context *context;
	bool recv_channels;
	bool send_channels;
	struct socket_set listen;
	struct socket_set init;
	struct socket_set online;
};

static int context_add_socket(const struct oh_context *c, oh_socket s)
{
	int events = oh_socket_events(s, 0);
	assert(events);
	return c->add_socket(s, events, c->data);
}

static int context_remove_socket(const struct oh_context *c, oh_socket s)
{
	return c->remove_socket(s, c->data);
}

oh_endpoint oh_endpoint_create(const struct oh_context *c, bool recv_channels, bool send_channels)
{
	oh_endpoint e = malloc(sizeof (oh_endpoint));
	if (e) {
		e->context = c;
		e->recv_channels = recv_channels;
		e->send_channels = send_channels;
		objset_init(&e->listen);
		objset_init(&e->init);
		objset_init(&e->online);
	}

	return e;
}

void oh_endpoint_destroy(oh_endpoint e)
{
	if (e) {
		oh_socket s;
		struct objset_iter i;

		for (s = objset_first(&e->online, &i); s; s = objset_next(&e->online, &i))
			socket_destroy(s);

		for (s = objset_first(&e->init, &i); s; s = objset_next(&e->init, &i))
			socket_destroy(s);

		for (s = objset_first(&e->listen, &i); s; s = objset_next(&e->listen, &i))
			socket_destroy(s);

		free(e);
	}
}

const struct oh_context *oh_endpoint_context(oh_endpoint e)
{
	return e->context;
}

void endpoint_config(oh_endpoint e, bool *recv_channels, bool *send_channels)
{
	*recv_channels = e->recv_channels;
	*send_channels = e->send_channels;
}

int oh_endpoint_bind(oh_endpoint e, const struct addrinfo *ai)
{
	if (!objset_empty(&e->listen)) {
		assert(false);
		errno = EBUSY;
		return -1;
	}

	int err = -1;

	for (; ai; ai = ai->ai_next) {
		int fd = system_socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
		if (fd < 0)
			goto no_fd;

		if (bind(fd, ai->ai_addr, ai->ai_addrlen) < 0)
			goto no_bind;

		if (listen(fd, SOMAXCONN) < 0)
			goto no_listen;

		oh_socket s = socket_create(e, fd);
		if (s == NULL)
			goto no_create;

		if (!objset_add(&e->listen, s))
			goto no_objset;

		socket_prepare_listen(s);

		if (context_add_socket(e->context, s) < 0)
			goto no_context;

		err = 0;
		continue;

	no_context:
		objset_del(&e->listen, s);
	no_objset:
		socket_destroy(s);
		continue;

	no_create:
	no_listen:
	no_bind:
		close(fd);
	no_fd:
		continue;
	}

	return err;
}

int oh_endpoint_connect(oh_endpoint e, const struct addrinfo *ai)
{
	int err = -1;

	for (; ai; ai = ai->ai_next) {
		int fd = system_socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
		if (fd < 0)
			goto no_fd;

		bool pending = false;

		if (connect(fd, ai->ai_addr, ai->ai_addrlen) < 0) {
			if (errno != EINPROGRESS)
				goto no_connect;

			pending = true;
		}

		oh_socket s = socket_create(e, fd);
		if (s == NULL)
			goto no_create;

		if (!objset_add(&e->init, s))
			goto no_objset;

		if (pending)
			socket_prepare_connect(s);
		else
			socket_prepare_handshake(s);

		if (context_add_socket(e->context, s) < 0)
			goto no_context;

		err = 0;
		continue;

	no_context:
		objset_del(&e->init, s);
	no_objset:
		socket_destroy(s);
		continue;

	no_create:
	no_connect:
		close(fd);
	no_fd:
		continue;
	}

	return err;
}

oh_message oh_endpoint_recv(oh_endpoint e)
{
	// TODO
	return NULL;
}

int oh_endpoint_send(oh_endpoint e, oh_message m)
{
	// TODO
	return -1;
}

int endpoint_socket_accept(oh_endpoint e, oh_socket s)
{
	for (int i = 0; i < SOMAXCONN; i++) {
		int fd = system_accept(oh_socket_fd(s), NULL, NULL);
		if (fd < 0) {
			if (errno == EAGAIN || errno == EINTR)
				break;

			goto no_fd;
		}

		oh_socket s = socket_create(e, fd);
		if (s == NULL)
			goto no_create;

		if (!objset_add(&e->init, s))
			goto no_objset;

		socket_prepare_handshake(s);

		if (context_add_socket(e->context, s) < 0)
			goto no_context;

		continue;

	no_context:
		objset_del(&e->init, s);
	no_objset:
		socket_destroy(s);
		return -1;

	no_create:
		close(fd);
	no_fd:
		return -1;
	}

	return 0;
}

int endpoint_socket_online(oh_endpoint e, oh_socket s)
{
	objset_del(&e->init, s);

	if (!objset_add(&e->online, s))
		return -1;

	return 0;
}

int endpoint_socket_message(oh_endpoint e, oh_socket s, oh_message m)
{
	if (TODO < 0)
		return -1;

	oh_message_ref(m);
	// TODO

	return 0;
}

void endpoint_socket_error(oh_endpoint e, oh_socket s)
{
	objset_del(&e->listen, s);
	objset_del(&e->init, s);
	objset_del(&e->online, s);

	socket_destroy(s);

	// TODO: reconnect etc.
}
