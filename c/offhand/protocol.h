#ifndef OFFHAND_PROTOCOL_H
#define OFFHAND_PROTOCOL_H

#include <stdint.h>

#include "packed.h"

#define PROTOCOL_VERSION  2

struct protocol_handshake {
	uint8_t version;
	uint8_t source_channel_id_size;
	uint8_t target_channel_id_size;
} PACKED;

struct protocol_prefix {
	uint64_t channel_id;
	uint16_t sequence;    // bit #15 is 1
} PACKED;

struct protocol_message_head {
	uint64_t channel_id;
	uint16_t sequence;    // bit #15 is 0
	uint16_t length;
	// blob size vector
	// blob data vector
} PACKED;

#endif
