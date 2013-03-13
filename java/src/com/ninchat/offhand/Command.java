package com.ninchat.offhand;

import java.nio.ByteBuffer;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

abstract class Command extends Receiver
{
	private static final Log log = LogFactory.getLog(Command.class);

	static final byte BEGIN    = 10;
	static final byte COMMIT   = 20;
	static final byte ROLLBACK = 30;

	Command()
	{
		buffer = ByteBuffer.allocate(1);
	}

	void receive(Consumer.Peer peer)
	{
		peer.channel.read(buffer, peer, this);
	}

	protected void received(Consumer.Peer peer)
	{
		log.debug("received");

		byte code = buffer.get(0);

		switch (code) {
		case BEGIN:
		case COMMIT:
		case ROLLBACK:
			if (!received(code)) {
				log.error("unexpected command from " + peer);
				peer.disconnect();
			}
			break;

		default:
			log.error("unknown command from " + peer);
			peer.disconnect();
			break;
		}
	}

	abstract boolean received(byte code);
}
