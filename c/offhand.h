#ifndef OFFHAND_H
#define OFFHAND_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifndef OH_API
#define OH_API
#endif

#define OH_EVENT_IN   1
#define OH_EVENT_OUT  2

struct addrinfo;

struct oh_blob;
struct oh_endpoint;
struct oh_message;
struct oh_socket;

typedef struct oh_blob *oh_blob;
typedef struct oh_endpoint *oh_endpoint;
typedef struct oh_message *oh_message;
typedef struct oh_socket *oh_socket;

struct oh_context {
	void *data;
	int (*add_socket)(oh_socket s, int events, void *data);
	int (*modify_socket)(oh_socket s, int events, void *data);
	int (*remove_socket)(oh_socket s, void *data);
};

OH_API oh_blob oh_blob_new(size_t size);
OH_API oh_blob oh_blob_ref(oh_blob b);
OH_API oh_blob oh_blob_unref(oh_blob b);
OH_API size_t oh_blob_size(oh_blob b);
OH_API void *oh_blob_data(oh_blob b);
OH_API void oh_blob_shrink(oh_blob b, size_t size);

OH_API oh_endpoint oh_endpoint_create(const struct oh_context *c, bool recv_channels, bool send_channels);
OH_API void oh_endpoint_destroy(oh_endpoint s);
OH_API const struct oh_context *oh_endpoint_context(oh_endpoint s);
OH_API int oh_endpoint_bind(oh_endpoint s, const struct addrinfo *ai);
OH_API int oh_endpoint_connect(oh_endpoint s, const struct addrinfo *ai);
OH_API oh_message oh_endpoint_recv(oh_endpoint s);
OH_API int oh_endpoint_send(oh_endpoint s, oh_message m);

OH_API oh_message oh_message_new(unsigned int length, const void *channel_data, size_t channel_size, const void *peer_data, size_t peer_size);
OH_API void oh_message_init(oh_message m, unsigned int index, oh_blob b);
OH_API oh_message oh_message_ref(oh_message m);
OH_API oh_message oh_message_unref(oh_message m);
OH_API size_t oh_message_peer_size(oh_message m);
OH_API const void *oh_message_peer_data(oh_message m);
OH_API size_t oh_message_channel_size(oh_message m);
OH_API const void *oh_message_channel_data(oh_message m);
OH_API unsigned int oh_message_length(oh_message m);
OH_API oh_blob oh_message_blob(oh_message m, unsigned int index);

OH_API int oh_socket_fd(oh_socket s);
OH_API int oh_socket_events(oh_socket s, int events);  // returns possibly updated event mask
OH_API void oh_socket_error(oh_socket s);

#ifdef __cplusplus
}
#endif

#endif
