
Offhand Protocol
================

This document specifies the Offhand network protocol version 1 (**DRAFT**).


## 1. General

### 1.1. Introduction

The protocol is intended for use in private networks, where the peers are
controlled by trusted parties.  It can be layered on any bi-directional binary
stream transport (e.g. TCP or TLS), and is specifically designed to cope with
connection failures.


### 1.2. Glossary

channel

> A half-duplex message stream within a link.

connection

> Single data transfer session of the underlying transport mechanism.  E.g. a
> TCP connection.

connector

> A peer which initiates connections.

connector's channel

> A message stream sent by a connector to a listener.

epoch

> The initialization time of a listener instance in microseconds since January
> 1, 1970 UTC.

link

> Logical pairing of peers, spanning multiple connections.

listener

> A peer which accepts connections, probably at a well-known network address.

listener instance

> An Offhand application's runtime state, including current links.

listener's channel

> A stream of messages sent by a listener to a connector.

message

> High-level payload, consisting of multiple parts.

peer

> One of the two linked Offhand applications.

sequence

> Message identifier within a sliding window in a channel.

transaction

> Mechanism for ensuring the successful processing of messages (in addition to
> the successful receiving of messages).


### 1.3. Requirements terminology

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD",
"SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be
interpreted as described in [RFC 2119].


### 1.4. Overall technical requirements

All integer values (everything except variable-length data) are encoded in
little-endian byte order on the wire (note: not in the network byte order).
Unused bits and padding bytes MUST be sent as zero, and ignored when received.
Unused enumeration or out-of-range values MUST NOT be sent, and the connection
MUST be retired when such a value is received.

Retiring a connection means that it MUST NOT be used to send packets, and it
SHOULD be terminated.


## 2. Handshake

Both peers send handshake data immediately after a connection has been
established.

Fields sent by the connector:

     Handshake field               | Size (bytes)
    :------------------------------|:--------------
     connector's version           | 1
     (padding)                     | 3
     handshake flags               | 1
     (padding)                     | 1
     connector's channel id size   | 1
     listener's channel id size    | 1
     old epoch (or zero)           | 8
     old link id (or zero)         | 8

Fields sent by the listener:

     Handshake field               | Size (bytes)
    :------------------------------|:--------------
     listener's version            | 1
     (padding)                     | 7
     old or new epoch              | 8
     old or new link id (or zero)  | 8

Handshake flags:

      Bit | Handshake flag
    -----:|:---------------------------------------
        0 | connector's channel uses transactions
        1 | listener's channel uses transactions
        2 | require old link id

A peer sends the highest version number it supports.  If the version numbers
don't match, the connection MUST be retired.  If the listener disagrees about
the transaction flags or channel id sizes, it MUST retire the connection.

When non-zero, an epoch identifies a listener instance at a given location.  If
it changes, it means that the link has been lost.

When non-zero, a link id identifies a link within a listener instance.  Its
value MUST be within the interval [1..2<sup>63</sup>[.

If the connector sent an unknown or zero-value link id (or the epoch has
changed), the listener replies with a new, unique link id.  Otherwise the
listener replies with the connector's link id to acknowledge it.

If the old link id required (see flags) but not known to the listener (or the
epoch has changed), it MUST reply with zero-value link id and retire the
connection.

If the connector receives a new link id (or a new epoch), state specific to the
old link id (if any) MUST be discarded, and messaging may start immediately.
If the peers agree on an old link id, packet exchange may start, but messaging
MUST NOT be started on a channel until it has been resumed.

Wire format of the connector's transmission:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +---------------+---------------+---------------+---------------+
    |  connector's  |                                               |
    |    version    |                                               |
    +---------------+---------------+---------------+---------------+
    |   handshake   |               |  connector's  |  listener's   |
    |     flags     |               | channel size  | channel size  |
    +---------------+---------------+---------------+---------------+
    |                           old epoch
    |
    +---------------+---------------+---------------+---------------+
                                 (cont'd)                           |
                                                                    |
    +---------------+---------------+---------------+---------------+
    |                              old
    |                            link id
    +---------------+---------------+---------------+---------------+
                                 (cont'd)                           |
                                                                    |
    +---------------+---------------+---------------+---------------+

Wire format of the listener's transmission:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +---------------+---------------+---------------+---------------+
    |  listener's   |
    |   version     |
    +---------------+---------------+---------------+----------------
                                                                    |
                                                                    |
    +---------------+---------------+---------------+---------------+
    |                           old or new
    |                             epoch
    +---------------+---------------+---------------+---------------+
                                 (cont'd)                           |
                                                                    |
    +---------------+---------------+---------------+---------------+
    |                           old or new
    |                            link id
    +---------------+---------------+---------------+---------------+
                                 (cont'd)                           |
                                                                    |
    +---------------+---------------+---------------+---------------+


## 3. Packets

### 3.1. Packet header

All packets start with the common packet header, and are aligned to an 8-byte
boundary by padding at the end.

The header consists of two bytes which contain the following fields:

     Header field                 | Byte  | Bits
    :-----------------------------|:------|:------
     channel                      | 0     | 0
     channel multicast            | 0     | 1
     channel packet format        | 0     | 2-4
     packet format-specific data  | 0     | 5-7
     short message length         | 1     | 0-7

The packet format depends on the channel bit and the channel packet format:

- General packets don't have the channel bit set.  The channel multicast,
  channel packet format and short message length fields are not used.

- Channel packets have the channel bit set.  The channel multicast bit
  specifies either unicasting or multicasting.  Channel packet formats:

      Value | Channel packet format     | Channel id space
    -------:|:--------------------------|:------------------
          0 | channel operation         | sender's
          1 | channel acknowledgement   | receiver's
          2 | sequence operation        | sender's
          3 | sequence acknowledgement  | receiver's
          4 | message                   | sender's

  The least-significant bit of the format value indicates the channel's
  direction.  The short message length field is used only with a specific
  message packet configuration.


### 3.2. General packet format

One of the following packet types is stored in the packet format-specific data
in the common packet header:

      Value | General packet type
    -------:|:---------------------
          0 | nop
          1 | ping
          2 | pong
          3 | resume
          4 | shutdown

Wire format:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-------+-----+---------------+---------------+---------------+
    |c|       |packe|
    | |       |type |
    +-+-------+-----+---------------+---------------+---------------+
                                                                    |
                                                                    |
    +---------------+---------------+---------------+---------------+

The nop (no operation) packet consists of 8 zero bytes, so it can be used for
additional padding between between packets.


### 3.3. Channel packet formats

Unicast packets contain the following field:

     Channel field      | Size (bytes)
    :-------------------|:----------------------------------------
     channel id         | _channel id size_

Multicast packets contain the following fields:

     Channel field      | Size (bytes)
    :-------------------|:----------------------------------------
     (padding)          | 2
     channel id count   | 4
     channel id vector  | _channel id size_ * _channel id count_

The channel id size was negotiated in the handshake, and is not repeated in
packets.  Note that when channel id size is zero, no channel id data is
transmitted.

If a channel id is specified multiple times, the message will be delivered
multiple times to the channel.  This is possible also when channel id size is
zero.

The theoretical maximum number of multicast targets is 2<sup>32</sup>-1, but
implementations MAY impose a stricter limit.


#### 3.3.1. Channel operation and acknowledgement packet format

One of the following packet types is stored in the packet format-specific data
in the common packet header:

      Value | Channel operation packet type
    -------:|:-------------------------------------
          0 | commit
          1 | rollback
          2 | close

      Value | Channel acknowledgement packet type
    -------:|:-------------------------------------
          0 | received
          1 | consumed
          2 | committed
          3 | uncommitted
          4 | closed

The message-specific packet types target the message which follows the message
targeted previously, either by the same packet type or the corresponding
sequence packet type.

Wire format (unicast, with a 2-byte channel id):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-----+-----+---------------+---------------+---------------+
    |c|m|packe|packe|               |            channel            |
    | | |forma|type |               |              id               |
    +-+-+-----+-----+---------------+---------------+---------------+
    |                                                               |
    |                                                               |
    +---------------+---------------+---------------+---------------+


#### 3.3.2. Sequence operation and acknowledgement packet format

The sequence packets contain the following field:

     Sequence field   | Size (bytes)
    :-----------------|:--------------
     (padding)        | 0..3
     sequence number  | 4

The sequence number is aligned to an 4-byte boundary, with the amount of
padding depending on the preceding fields.

One of the following packet types is stored in the format-specific data in the
common packet header:

      Value | Sequence operation packet type
    -------:|:--------------------------------------
          0 | commit
          1 | rollback

      Value | Sequence acknowledgement packet type
    -------:|:--------------------------------------
          0 | received
          1 | consumed
          2 | committed
          3 | uncommitted

The sequence number indicates the range of targeted messages.

Wire format (unicast, with an 8-byte channel id):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-----+-----+---------------+---------------+---------------+
    |c|m|packe|packe|               |            channel
    | | |forma|type |               |              id
    +-+-+-----+-----+---------------+---------------+---------------+
                                 (cont'd)

    +---------------+---------------+---------------+---------------+
                 (cont'd)           |                               |
                                    |                               |
    +---------------+---------------+---------------+---------------+
    |                            sequence                           |
    |                             number                            |
    +---------------+---------------+---------------+---------------+


#### 3.3.3. Message packet formats

The packet format-specific data in the common packet header contains the
following message flags:

      Bit | Message flag
    -----:|:--------------
        0 | long
        1 | large

All message packets contain the following field:

     Message field              | Size (bytes)
    :---------------------------|:--------------------------------
     (padding)                  | 0..1

In other words, the message fields are aligned to a 2-byte boundary, with the
amount of padding depending on the preceding fields.


##### 3.3.3.1. Message length

When the long message flag is not set, the short message length field of the
common packet header is used.

When the long message flag is set, the message packet contains the following
field:

     Message field              | Size (bytes)
    :---------------------------|:--------------------------------
     long message length        | 4

The vector length is either an 8-bit or a 32-bit value; while the theoretical
maximum length is 2<sup>32</sup>-1, implementations MAY impose a stricter limit.


##### 3.3.3.2. Message payload size

When the large data flag is not set, the message packet contains the following
fields:

     Message field              | Size (bytes)
    :---------------------------|:--------------------------------
     small payload size vector  | 2 * _message length_

When the large data flag is set, the message packet contains the following
fields:

     Message field              | Size (bytes)
    :---------------------------|:--------------------------------
     (padding)                  | 0..6
     large payload size vector  | 8 * _message length_

The large size vector is aligned to an 8-byte boundary, with the amount of
padding depending on the preceding fields.

The sizes are either 16-bit or 64-bit values.  While the theoretical maximum
size is 2<sup>64</sup>-1, implementations MAY impose a stricter limit.


##### 3.3.3.3. Message payload data

Message packets contain the following field:

     Message field              | Size (bytes)
    :---------------------------|:--------------------------------
     (padding)                  | 0..6
     payload data vector        | _sum of aligned payload sizes_

In case of small size vector, padding may be required to align the start of the
data vector to an 8-byte bounary.  The parts of the data vector are aligned to
8-byte boundaries by padding at the end.


##### 3.3.3.4. Message packet examples

Wire format of a short, small message (unicast, with a 1-byte channel id, and a
3-part payload with 3-, 0- and 5-byte payload parts):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-----+-+-+-+---------------+---------------+---------------+
    |c|m|packe|l|l| | short message |    channel    |               |
    | | |forma|v|p| |    length     |      id       |               |
    +-+-+-----+-+-+-+---------------+---------------+---------------+
    |         small payload         |         small payload         |
    |            size #0            |            size #1            |
    +---------------+---------------+---------------+---------------+
    |         small payload         |
    |            size #2            |
    +---------------+---------------+---------------+---------------+
                                                                    |
                                                                    |
    +---------------+---------------+---------------+---------------+
    |                    payload                    |
    |                    data #0                    |
    +---------------+---------------+---------------+---------------+
                                                                    |
                                                                    |
    +---------------+---------------+---------------+---------------+
    |                            payload
    |                            data #2
    +---------------+---------------+---------------+---------------+
        (cont'd)    |                                               |
                    |                                               |
    +---------------+---------------+---------------+---------------+


Wire format of a long, large message (multicast to 2 channels, with 4-byte
channel ids and a 2-part payload, excluding payload data vector for brevity):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-----+-+-+-+---------------+---------------+---------------+
    |c|m|packe|l|l|                                                 |
    | | |forma|v|p|                                                 |
    +-+-+-----+-+-+-+---------------+---------------+---------------+
    |                           channel id                          |
    |                             count                             |
    +---------------+---------------+---------------+---------------+
    |                           channel id                          |
    |                              #0                               |
    +---------------+---------------+---------------+---------------+
    |                           channel id                          |
    |                              #1                               |
    +---------------+---------------+---------------+---------------+
    |                         large payload                         |
    |                            length                             |
    +---------------+---------------+---------------+---------------+
    |                                                               |
    |                                                               |
    +---------------+---------------+---------------+---------------+
    |                         large payload
    |                            size #0
    +---------------+---------------+---------------+---------------+
                                 (cont'd)                           |
                                                                    |
    +---------------+---------------+---------------+---------------+
    |                         large payload
    |                            size #1
    +---------------+---------------+---------------+---------------+
                                 (cont'd)                           |
                                                                    |
    +---------------+---------------+---------------+---------------+


## 4. Functions

### 4.1. Messaging

Messages have an implicit 32-bit sequence number, starting at 0, incrementing
monotonically, and wrapping from 2<sup>32</sup>-1 to 0.  Theoretically there
can be up to 2<sup>32</sup>-1 outstanding messages at a given time, but an
implementation MAY impose a stricter limit.  The receiver needs to acknowledge
consumed messages to prevent blocking.

Multicasting assigns different sequence number for each channel, and the
acknowledgements need to be done for each channel.


### 4.2. Transactional messaging

When transactions are enabled, the sender requests received messages to be
processed (commit op) or ignored (rollback op) by the receiver.  In case of
commit, the receiver indicates success (committed ack) or failure (uncommitted
ack).

~~Since a rollback doesn't have to be acknowledged, the next commit MUST be
acknowledged by specifying the sequence number explicitly.  This design is
necessary because the rollback may have been missed in case of reconnection,
and to avoid taxing slow links which may receive lots of rollbacks.~~


### 4.3. Channel closure

Sender notifies the receiver that it is closing a channel (close op).  Receiver
acknowledges the closure (closed ack).  After this, the channel may be reopened
by the sender (by sending a message).


### 4.4. Health check

Ping packets MAY be sent at any time.  A pong packet SHOULD be sent when a ping
packet is received, unless the connection is closed before it's possible.  A
pong MUST NOT be queued to be sent after a reconnection.


### 4.5. Reconnection

A peer sends the resume packet when:

1. an old link id was negotiated during the handshake; and
2. the peer has notified the other peer about all received, consumed and
   (optionally) committed/uncommitted messages.


### 4.6. Link shutdown

The shutdown packet means that the sender isn't going to open any more
channels.  The connection SHOULD be terminated, the link state SHOULD be
discarded and reconnecting MUST be stopped after both peers have sent a
shutdown packet.


[RFC 2119]: https://www.ietf.org/rfc/rfc2119.txt

