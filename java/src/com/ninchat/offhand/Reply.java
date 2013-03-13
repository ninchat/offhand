package com.ninchat.offhand;

import java.nio.ByteBuffer;
import java.nio.channels.CompletionHandler;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

abstract class Reply implements CompletionHandler<Integer, Consumer.Peer>
{
	private static final Log log = LogFactory.getLog(Reply.class);

	static final byte RECEIVED = 11;
	static final byte ENGAGED  = 21;
	static final byte CANCELED = 22;

	void send(Consumer.Peer peer, byte code)
	{
		ByteBuffer buffer = ByteBuffer.allocate(1);
		buffer.put(code);
		buffer.rewind();
		peer.channel.write(buffer, peer, this);
	}

	public void completed(Integer result, Consumer.Peer peer)
	{
		int len = result.intValue();
		if (len > 0) {
			log.debug("sent");
			sent();
		} else if (len < 0) {
			log.warn("EOF while sending to " + peer);
			peer.disconnect();
		}
	}

	public void failed(Throwable e, Consumer.Peer peer)
	{
		log.error("failed to send to " + peer, e);
		peer.disconnect();
	}

	abstract void sent();
}
