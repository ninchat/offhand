#include "internal.h"

#include <assert.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>

#include <sys/socket.h>
#include <sys/types.h>
#include <sys/uio.h>
#include <limits.h>
#include <unistd.h>

#include <ccan/endian/endian.h>

#include "packed.h"
#include "protocol.h"

enum in_state {
	IN_LISTEN = 1,
	IN_HANDSHAKE,
	IN_PREFIX,
	IN_MESSAGE_HEAD,
	IN_MESSAGE_BODY,
};

enum out_state {
	OUT_CONNECT = 1,
	OUT_HANDSHAKE,
	OUT_MESSAGE,
	OUT_ACK,
};

struct oh_socket {
	oh_endpoint endpoint;

	int fd;

	enum in_state in_state;
	enum out_state out_state;

	uint64_t in_offset;
	uint64_t out_offset;

	oh_message in_message;
	oh_message out_message;

	uint8_t in_buffer[IN_BUFFER_SIZE];


	union {
		struct protocol_handshake in_handshake;

		uint16_t in_prefix;
		struct protocol_message_head in_prefix_padding;

		struct {
			oh_blob in_message_head;
			oh_message in_message_body;
		};
	};

	union {
		struct protocol_handshake out_handshake;

		struct {
			oh_blob out_message_head;
			oh_message out_message_body;
		};

		uint16_t out_ack;
	};
};

static void socket_in_reset(oh_socket s)
{
	switch (s->in_state) {
	case IN_MESSAGE_HEAD:
		oh_blob_unref(&s->in_message_head);
		break;

	case IN_MESSAGE_BODY:
		oh_blob_unref(&s->in_message_head);
		oh_message_destroy(s->in_message_body);
		break;

	default:
		break;
	}

	s->in_state = 0;
	s->in_offset = 0;
}

static void socket_out_reset(oh_socket s)
{
	switch (s->out_state) {
	case OUT_MESSAGE:
		oh_blob_unref(&s->out_message_head);
		oh_message_destroy(s->out_message_body);
		break;

	default:
		break;
	}

	s->out_state = 0;
	s->out_offset = 0;
}

#define IO(func, dir, fd, buf, bufsize, offserptr)              \
                                                                \
	void *ptr = (void *) buf + *(offsetptr);                \
	ssize_t remain = (ssize_t) bufsize - *(offsetptr);      \
	assert(remain > 0);                                     \
                                                                \
	ssize_t len = func(fd, ptr, remain);                    \
	if (len < 0) {                                          \
		if (errno == EAGAIN || errno == EINTR)          \
			return 0;                               \
                                                                \
		return -1;                                      \
	}                                                       \
                                                                \
	if (len == 0) {                                         \
		errno = 0;                                      \
		return -1;                                      \
	}                                                       \
                                                                \
	*(offsetptr) += len;                                    \
                                                                \
	return 0;

static int socket_read_in_linear(oh_socket s, void *buf, size_t bufsize)
{
	IO(read, in, s->fd, buf, bufsize, &s->in_offset)
}

static int socket_write_out_linear(oh_socket s, void *buf, size_t bufsize)
{
	IO(write, out, s->fd, buf, bufsize, &s->out_offset)
}

static int socket_read_in_message(oh_socket s, oh_message m)
{
	unsigned int message_length = oh_message_length(m);

	unsigned int base_index = 0;
	oh_blob base_blob;
	uint32_t base_blob_remain;

	for (uint64_t offset = 0; base_index < message_length; base_index++) {
		base_blob = oh_message_peek(m, base_index);
		offset += oh_blob_size(base_blob);

		if (offset > s->in_offset) {
			base_blob_remain = offset - s->in_offset;
			goto out;
		}
	}

	return 0;

out:
	;

	unsigned int vector_length = message_length - base_index;
	if (vector_length > IOV_MAX)
		vector_length = IOV_MAX;

	struct iovec vector[vector_length];

	vector[0].iov_base = oh_blob_data(base_blob);
	vector[0].iov_len = base_blob_remain;

	for (unsigned int i = 0; i < vector_length; i++) {
		oh_blob b = oh_message_peek(m, base_index + i);
		vector[i].iov_base = oh_blob_data(b);
		vector[i].iov_len = oh_blob_size(b);
	}

	ssize_t len = readv(s->fd, vector, vector_length);
	if (len < 0) {
		if (errno == EAGAIN || errno == EINTR)
			return 0;

		return -1;
	}

	if (len == 0) {
		errno = 0;
		return -1;
	}

	s->in_offset += len;

	return 0;
}

oh_socket socket_create(oh_endpoint e, int fd)
{
	oh_socket s = calloc(1, sizeof (oh_socket));
	if (s) {
		s->endpoint = e;
		s->fd = fd;
	}

	return s;
}

void socket_destroy(oh_socket s)
{
	if (s) {
		socket_in_reset(s);
		socket_out_reset(s);

		if (s->fd >= 0)
			close(s->fd);

		free(s);
	}
}

int oh_socket_fd(oh_socket s)
{
	return s->fd;
}

void socket_prepare_listen(oh_socket s)
{
	s->in_state = STATE_LISTEN;
}

void socket_prepare_connect(oh_socket s)
{
	s->out_state = STATE_CONNECT;
}

static int socket_complete_connect(oh_socket s)
{
	int err;
	socklen_t errlen = sizeof (err);

	if (getsockopt(s->fd, SOL_SOCKET, SO_ERROR, &err, &errlen) < 0)
		return -1;

	if (err) {
		if (err == EALREADY)
			return 0;

		errno = err;
		return -1;
	}

	socket_out_reset(s);

	socket_prepare_handshake(s);

	return 0;
}

void socket_prepare_handshake(oh_socket s)
{
	bool recv_channels;
	bool send_channels;

	endpoint_config(s->endpoint, &recv_channels, &send_channels);

	s->in_state = STATE_HANDSHAKE;

	s->out_state = STATE_HANDSHAKE;
	s->out_handshake.version = PROTOCOL_VERSION;
	s->out_handshake.source_channel_id_size = send_channels ? sizeof (uint64_t) : 0;
	s->out_handshake.target_channel_id_size = recv_channels ? sizeof (uint64_t) : 0;
}

static int socket_complete_handshake(oh_socket s)
{
	if (s->in_offset != sizeof (struct protocol_handshake) ||
	    s->out_offset != sizeof (struct protocol_handshake))
		return 0;

	if (s->in_handshake.version != s->out_handshake.version) {
		errno = EPROTONOSUPPORT;
		return -1;
	}

	if (s->in_handshake.source_channel_id_size != s->out_handshake.target_channel_id_size ||
	    s->in_handshake.target_channel_id_size != s->out_handshake.source_channel_id_size) {
		errno = EBADMSG;
		return -1;
	}

	socket_in_reset(s);
	socket_out_reset(s);

	if (endpoint_socket_online(s->endpoint, s) < 0)
		return -1;

	s->in_state = STATE_RECV_PREFIX;

	// TODO: ask endpoint for stuff to send

	return 0;
}

static int socket_recv_handshake(oh_socket s)
{
	if (socket_read_linear(s, &s->in_handshake, sizeof (struct protocol_handshake)) < 0)
		return -1;

	if (s->in_offset < sizeof (struct protocol_handshake))
		return 0;

	return socket_complete_handshake(s);
}

static int socket_send_handshake(oh_socket s)
{
	if (socket_write_linear(s, &s->out_handshake, sizeof (struct protocol_handshake)) < 0)
		return -1;

	if (s->out_offset < sizeof (struct protocol_handshake))
		return 0;

	return socket_complete_handshake(s);
}

static ssize_t socket_consume_prefix(oh_socket s, void *buf, size_t bufsize)
{
	uint16_t prefix = LE16_TO_CPU(*(uint16_t *) buf);

	if (prefix & 0x8000) {
		uint16_t sequence = prefix & 0x7fff;

		// TODO: handle ack

		return sizeof (uint16_t);
	} else {
		uint16_t message_length = prefix;
		uint32_t vector_size = sizeof (uint32_t) * message_length;

		oh_blob b = oh_blob_new(sizeof (struct protocol_message_head) + vector_size);
		if (b == NULL)
			return -1;

		s->in_state = STATE_RECV_MESSAGE_HEAD;
		s->in_message_head = b;

		return bufsize;
	}
}

static int socket_recv_prefix(oh_socket s)
{
	if (socket_read_linear(s, &s->in_prefix, sizeof (s->in_prefix)) < 0)
		return -1;

	if (s->in_offset < sizeof (uint16_t))
		return 0;

	void *buf = &s->in_prefix;
	ssize_t bufsize = s->in_offset;

	s->in_offset = 0;

	while (bufsize > 0) {
		if (bufsize < sizeof (uint16_t)) {
			memmove(&s->in_prefix, buf, bufsize);
			s->in_offset = bufsize;
			break;
		}

		ssize_t len = socket_consume_prefix(s, buf, bufsize);
		if (len < 0)
			return -1;

		buf += len;
		bufsize -= len;
	}

	return 0;
}

static int socket_recv_message_head(oh_socket s)
{
	if (socket_read_linear(s, oh_blob_data(s->in_message_head), oh_blob_size(s->in_message_head)) < 0)
		return -1;

	uint32_t head_size = oh_blob_size(s->in_message_head);

	if (s->in_offset < head_size)
		return 0;

	uint32_t vector_size = head_size - sizeof (struct protocol_message_head);
	unsigned int message_length = vector_size / sizeof (uint32_t);

	struct protocol_message_head *head = oh_blob_data(s->in_message_head);
	uint32_t *vector = (uint32_t *) (head + 1);

	oh_message m = oh_message_create(message_length);
	if (m == NULL)
		goto no_message;

	oh_message_set_channel(m, head->channel_id);

	for (unsigned int i = 0; i < message_length; i++) {
		uint32_t blob_size = LE32_TO_CPU(vector[i]);

		oh_blob b = oh_blob_new(blob_size);
		if (b == NULL)
			goto no_blob;

		oh_message_assign(m, i, b);
		oh_blob_unref(&b);
	}

	// don't reset in

	s->in_state = STATE_RECV_MESSAGE_BODY;
	s->in_message = m;

	return 0;

no_blob:
	oh_message_destroy(m);
no_message:
	return -1;
}

static int socket_recv_message_body(oh_socket s)
{
	if (read_message(s, s->in_message_body) < 0)
		return -1;

	if (s->in_offset < oh_message_size(s->in_message_body))
		return 0;

	oh_message m = s->in_message_body;
	s->in_message_body = NULL;

	socket_in_reset(s);

	if (endpoint_socket_message(s->endpoint, s, m) < 0)
		return -1;

	s->in_state = STATE_RECV_PREFIX;

	return 0;
}

int oh_socket_events(oh_socket s, int events)
{
	int err;

	if (events & OH_EVENT_IN) {
		switch (s->in_state) {
		case STATE_LISTEN:
			err = endpoint_socket_accept(s->endpoint, s);
			break;

		case STATE_HANDSHAKE:
			err = socket_recv_handshake(s);
			break;

		case STATE_RECV_PREFIX:
			err = socket_recv_prefix(s);
			break;

		case STATE_RECV_MESSAGE_HEAD:
			err = socket_recv_message_head(s);
			break;

		case STATE_RECV_MESSAGE_BODY:
			err = socket_recv_message_body(s);
			break;

		default:
			assert(false);
			errno = EINVAL;
			err = -1;
			break;
		}

		if (err < 0)
			return -1;
	}

	if (events & OH_EVENT_OUT) {
		switch (s->out_state) {
		case STATE_CONNECT:
			err = socket_complete_connect(s);
			break;

		case STATE_HANDSHAKE:
			err = socket_send_handshake(s);
			break;

		// TODO

		default:
			assert(false);
			errno = EINVAL;
			err = -1;
			break;
		}

		if (err < 0)
			return -1;
	}

	int flags = 0;

	if (s->in_state)
		flags |= OH_EVENT_IN;

	if (s->out_state)
		flags |= OH_EVENT_OUT;

	return flags;
}

void oh_socket_error(oh_socket s)
{
	endpoint_socket_error(s->endpoint, s);
}
