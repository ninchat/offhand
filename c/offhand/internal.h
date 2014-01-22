#define _GNU_SOURCE

#define OH_API  __attribute__ ((visibility ("default")))

#include <sys/types.h>

#include "offhand.h"

typedef struct buffer buffer;

struct buffer;

buffer buffer_create(size_t channel_id_size);
void buffer_destroy(buffer b);
void buffer_reset(buffer b);

ssize_t buffer_in_read(buffer b, int fd);
size_t buffer_in_available1(buffer b);
size_t buffer_in_available2(buffer b);
const void *buffer_in_data1(buffer b);
const void *buffer_in_data2(buffer b);
void buffer_in_consume(buffer b, size_t size);

void *buffer_out_ptr(buffer b, size_t size);
void buffer_out_produce(buffer b, size_t size);
ssize_t buffer_out_write(buffer b, int fd);

void endpoint_config(oh_endpoint e, bool *recv_channels, bool *send_channels);
int endpoint_socket_accept(oh_endpoint e, oh_socket s);
int endpoint_socket_online(oh_endpoint e, oh_socket s);
int endpoint_socket_message(oh_endpoint e, oh_socket s, oh_message m);
void endpoint_socket_error(oh_endpoint e, oh_socket s);

oh_socket socket_create(oh_endpoint e, int fd);
void socket_destroy(oh_socket s);
void socket_prepare_listen(oh_socket s);
void socket_prepare_connect(oh_socket s);
void socket_prepare_handshake(oh_socket s);
