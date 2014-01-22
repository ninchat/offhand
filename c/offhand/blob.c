#include "internal.h"

#include <assert.h>
#include <stdlib.h>

#include "atomic.h"

struct oh_blob {
	unsigned long refcount;
	uint32_t size;
};

oh_blob oh_blob_new(uint32_t size)
{
	oh_blob b = malloc(sizeof (oh_blob) + size);
	if (b) {
		b->refcount = 1;
		b->size = size;
	}
	return b;
}

oh_blob oh_blob_ref(oh_blob b)
{
	atomic_inc(&b->refcount);
	return b;
}

oh_blob oh_blob_unref(oh_blob b)
{
	if (b && atomic_dec(&b->refcount) == 1)
		free(b);

	return NULL;
}

void oh_blob_shrink(oh_blob b, uint32_t size)
{
	assert(size <= b->size);
	b->size = size;
}

uint32_t oh_blob_size(oh_blob b)
{
	return b->size;
}

void *oh_blob_data(oh_blob b)
{
	return b + 1;
}
