#include "internal.h"

#include <assert.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include "atomic.h"

struct oh_message {
	uint64_t channel;
	uint16_t length;
};

static oh_blob *message_vector(oh_message m)
{
	return (oh_blob *) (m + 1);
}

oh_message oh_message_new(uint16_t length)
{
	size_t vector_size = sizeof (oh_blob *) * length;
	oh_message m = malloc(sizeof (oh_message) + vector_size);
	if (m) {
		m->refcount = 1;
		m->length = length;
		memset(message_vector(m), 0, vector_size);
	}
	return m;
}

oh_message oh_message_ref(oh_message m)
{
	atomic_inc(&m->refcount);
	return m;
}

oh_message oh_message_unref(oh_message m)
{
	if (m && atomic_dec(&m->refcount) == 1) {
		for (unsigned int i = 0; i < m->length; i++)
			oh_blob_unref(&message_vector(m)[i]);

		free(m);
	}

	return NULL;
}

void oh_message_set_channel(oh_message m, uint64_t channel)
{
	m->channel = channel;
}

uint64_t oh_message_get_channel(oh_message m)
{
	return m->channel;
}

uint16_t oh_message_length(oh_message m)
{
	return m->length;
}

size_t oh_message_size(oh_message m)
{
	size_t size = 0;

	for (unsigned int i = 0; i < m->length; i++)
		size += oh_blob_size(message_vector(m)[i]);

	return size;
}

void oh_message_assign(oh_message m, uint16_t i, oh_blob b)
{
	assert(i < m->length);
	if (b)
		oh_blob_ref(b);
	oh_blob *v = message_vector(m);
	oh_blob_unref(&v[i]);
	v[i] = b;
}

oh_blob oh_message_peek(oh_message m, uint16_t i)
{
	assert(i < m->length);
	return message_vector(m)[i];
}
