package com.ninchat.offhand;

import java.nio.ByteBuffer;
import java.nio.channels.CompletionHandler;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

abstract class Receiver implements CompletionHandler<Integer, Consumer.Peer>
{
	private static final Log log = LogFactory.getLog(Receiver.class);

	protected ByteBuffer buffer;

	public void completed(Integer result, Consumer.Peer peer)
	{
		if (buffer.hasRemaining()) {
			log.warn("EOF from " + peer);
			peer.disconnect();
		} else {
			received(peer);
		}
	}

	public void failed(Throwable e, Consumer.Peer peer)
	{
		log.error("failed to receive from " + peer, e);
		peer.disconnect();
	}

	protected abstract void received(Consumer.Peer peer);
}
