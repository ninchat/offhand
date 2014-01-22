#include <assert.h>

#include "offhand.h"

int main(void)
{
	oh_blob b = oh_blob_new(10);
	assert(b);
	assert(oh_blob_size(b) == 10);
	assert(oh_blob_data(b));
	oh_blob b2 = oh_blob_ref(b);
	oh_blob_unref(b2);
	oh_message m = oh_message_new(1, NULL, 0, NULL, 0);
	assert(m);
	assert(oh_message_length(m) == 1);
	oh_message_init(m, 0, b);
	oh_blob_unref(b);
	oh_message_unref(m);
	return 0;
}
