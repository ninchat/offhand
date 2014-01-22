
Offhand Protocol Version 2
==========================

The protocol can be layered on any reliable bi-directional binary transport
(e.g. TCP, TLS).  It consists of an initial handshake, followed by full-duplex
packet exchange until the connection is terminated.  (As messaging is not
restricted to any client-server model or request-response pattern, such
terminology is avoided.)

All integer values described in this specification are encoded in little-endian
byte order on the wire (note: NOT in the network byte order); the least
significant bit is the leftmost in the diagrams.  Any unused bits must be sent
as zero and ignored when received.  Any unused enumeration values must not be
sent, and the connection should be terminated when one is received.


1. Handshake
------------

The "connector" refers to the peer which initiated the connection, and the
"listener" to the peer which accepted the connection (e.g. has a well-known
name).  The "connector channel" and the "listener channel" refer to the message
streams sent by the respective peer.

The peers exchange the following fields in the specified order (half-duplex):

     Order | Sender    | Field                     | Size (bytes)
    ------:|:----------|:--------------------------|:-------------
        1. | connector | connector version         | 1
        2. | listener  | listener version          | 1
        3. | connector | flags                     | 1
           |           | connector channel id size | 1
           |           | listener channel id size  | 1
           |           | old socket id size        | 1
           |           | old socket id data        | old socket id size
        4. | listener  | new socket id size        | 1
           |           | new socket id data        | new socket id size

Flags:

     Bit | Flag
    ----:|:------------------------------------
       0 | connector channel uses transactions
       1 | listener channel uses transactions

The connector sends the highest version number it supports, and the listener
replies with the same or a lower version number.  If either peer doesn't
support the other's version, the connection should be terminated.

After a mutually supported protocol version has been negotiated, the connector
sends the flags, channel id sizes and a socket id (which may be empty, i.e.
zero-length).  If the listener disagrees about the flags or channel id sizes,
it should terminate the connection.  If the connector sent an empty or unknown
socket id, the listener should reply with a unique (non-empty) socket id.
Otherwise the listener should reply with an empty socket id to acknowledge the
valid socket id sent by the connector.  (It's an error if both peers send an
empty socket id.)

If the connector receives a new socket id, any state specific to any old socket
id must be discarded, and messaging may start immediately on all channels.  If
the peers agree on an old socket id, packet exchange may start, but messaging
must not be started on a channel until it's resumed.

Wire format of the connector's transmission (with 2-byte socket id):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +---------------+---------------+---------------+---------------+
    |   connector   |     flags     |   connector   |   listener    |
    |    version    |               | channel size  | channel size  |
    +---------------+---------------+---------------+---------------+
    |  old socket   |          old socket           |
    |    id size    |           id data             |
    +---------------+ - - - - - - - - - - - - - - - +

Wire format of the listener's transmission (with 2-byte socket id):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +---------------+---------------+ - - - - - - - - - - - - - - - +
    |   listener    |  new socket   |          new socket           |
    |    version    |    id size    |           id data             |
    +---------------+---------------+ - - - - - - - - - - - - - - - +


2. Packets
----------

The common packet header consists of a single byte which contains the following
bit field:

     Field       | Bits
    :------------|:-----
     packet type | 0-3

Message content packet types:

     Type | Description
    -----:|:-------------------
        0 | small message
        1 | large message

Message control packet types:

     Type | Description
    -----:|:-------------------
        2 | messages received
        3 | commit messages
        4 | roll back messages
        5 | commit successful
        6 | commit failed

Resume packet type:

     Type | Description
    -----:|:-------------------
        7 | resume

Keep-alive packet types:

     Type | Description
    -----:|:-------------------
        8 | ping
        9 | pong


### 2.1. Message packets

Message content and control packets contain the following field:

     Field      | Size (bytes)
    :-----------|:----------------
     channel id | channel id size

The size of the channel id was negotiated in the handshake, and is not repeated
here.  (Note that it may be zero, which means that no channel id data is
transmitted.)


#### 2.1.1. Message content packets

Message content packets contain the following fields:

     Field               | Size (bytes)
    :--------------------|:---------------------------------------
     message length      | 1
     payload size vector | message length * 1; message length * 8
     payload data vector | sum of payload size vector

The payload size vector format depends on the packet type: small message
packets use 1-byte sizes and large messages use 8-byte sizes.  Hence messages
with maximum payload part sizes of 255 bytes or less can be encoded in compact
wire format, and the maximum supported payload part size is 2^64-1.

Wire format of a small message (with 2-byte channel id, and 3-part payload with
1-, 0- and 4-byte payload parts):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-------+-------+ - - - - - - - - - - - - - - - +---------------+
    |packet |unused |            channel            |    message    |
    | type  |       |              id               |    length     |
    +-------+-------+ - - - - - - - + - - - - - - - +---------------+
    |    payload    |    payload    |    payload    |    payload    |
    |    size #0    |    size #1    |    size #2    |    data #0    |
    + - - - - - - - + - - - - - - - + - - - - - - - + - - - - - - - +
    |                            payload                            |
    |                            data #2                            |
    + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +

Wire format of a large message (with 2-byte channel id and 2-part payload;
excluding payload data vector for brevity):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-------+-------+ - - - - - - - - - - - - - - - +---------------+
    |packet |unused |            channel            |    message    |
    | type  |       |              id               |    length     |
    +-------+-------+ - - - - - - - - - - - - - - - +---------------+
    |
    |                            payload
    + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
                                 size #0                            |
                                                                    |
    + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
    |
    |                            payload
    + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
                                 size #1                            |
                                                                    |
    + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +


#### 2.1.2. Message control packets

Message control packets contain the following field:

     Field           | Size (bytes)
    :----------------|:----------------
     sequence number | 2

Indicates a command or state change targetting one or more previously
transferred messages.  The sequence number indicates the range of targeted
messages.

Wire format (with 1-byte channel id):

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-------+-------+ - - - - - - - +-------------------------------+
    |packet |unused |    channel    |           sequence            |
    | type  |       |      id       |            number             |
    +-------+-------+ - - - - - - - +-------------------------------+


### 2.2. Resume packets

A peer sends the resume packet:

1. if an old socket id has been negotiated during the handshake; and
2. after it has notified the other peer about all of its known channels.


### 2.3. Keep-alive packets

- Ping packets may be sent by either peer.
- A single pong packet should be sent after receiving one or more ping packets.

