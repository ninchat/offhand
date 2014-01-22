#include "internal.h"

#include <assert.h>
#include <stdlib.h>

#include <sys/uio.h>
#include <unistd.h>

#define MAXSIZE_MESSAGE_HEADER  (2 + 2 + 8 * 1000)
#define MAXSIZE_ACK             2

struct buffer {
	size_t size;
	size_t begin;
	size_t end;
};

static void *buffer_memory(struct buffer *b)
{
	return b + 1;
}

buffer buffer_create(size_t channel_id_size)
{
	size_t size = channel_id_size + MAXSIZE_ACK + channel_id_size + MAXSIZE_MESSAGE_HEADER;
	buffer b = malloc(sizeof (struct buffer) + size);
	if (b) {
		b->size = size;
		buffer_reset(b);
	}
	return b;
}

void buffer_destroy(buffer b)
{
	free(b);
}

void buffer_reset(buffer b)
{
	b->begin = b->size;
	b->end = 0;
}

ssize_t buffer_in_read(buffer b, int fd)
{
	assert(b->begin != b->end);

	size_t real_begin = b->begin;
	if (real_begin == b->size)
		real_begin = 0;

	void *mem = buffer_memory(b);
	ssize_t len;

	if (real_begin == 0) {
		len = read(fd, mem + b->end, b->size - b->end);
	} else if (b->end < real_begin) {
		len = read(fd, mem + b->end, real_begin - b->end);
	} else {
		struct iovec vec[] = {
			{
				.iov_base = mem + b->end,
				.iov_len = b->size - b->end,
			},
			{
				.iov_base = mem,
				.iov_len = real_begin,
			},
		};

		len = readv(fd, vec, 2);
	}

	if (len > 0) {
		b->begin = real_begin;

		b->end += len;
		if (b->end >= b->size)
			b->end -= b->size;
	}

	return len;
}

size_t buffer_in_available1(buffer b)
{
	if (b->begin == b->size)
		return 0;
	else if (b->begin == b->end)
		return b->size;
	else if (b->begin < b->end)
		return b->end - b->begin;
	else
		return b->size - b->begin;
}

size_t buffer_in_available2(buffer b)
{
	if (b->begin == b->size || b->begin <= b->end)
		return 0;
	else
		return b->end;
}

const void *buffer_in_data1(buffer b)
{
	assert(b->begin < b->size);
	return buffer_memory(b) + b->begin;
}

const void *buffer_in_data2(buffer b)
{
	assert(b->begin > b->end && b->begin < b->size);
	return buffer_memory(b);
}

void buffer_in_consume(buffer b, size_t size)
{
	assert(b->begin < b->size);

	b->begin += size;
	if (b->begin >= b->size)
		b->begin -= b->size;

	if (b->begin == b->end)
		buffer_reset(b);
}

void *buffer_out_ptr(buffer b, size_t size)
{
	assert(b->end + size <= b->size);
	return buffer_memory(b) + b->end;
}

void buffer_out_produce(buffer b, size_t size)
{
	b->end += size;
	assert(b->end <= b->size);
}

ssize_t buffer_out_write(buffer b, int fd)
{
	if (b->begin == b->size)
		b->begin = 0;

	assert(b->begin < b->end);

	ssize_t len = write(fd, buffer_memory(b) + b->begin, b->end - b->begin);
	if (len > 0) {
		b->begin += len;

		if (b->begin == b->end)
			buffer_reset(b);
	}

	return len;
}
