package com.ninchat.offhand;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

abstract class Payload
{
	private static final Log log = LogFactory.getLog(Payload.class);

	void receive(Consumer.Peer peer)
	{
		Size size = new Size();
		peer.channel.read(size.buffer, peer, size);
	}

	private class Size extends Receiver
	{
		private Size()
		{
			buffer = ByteBuffer.allocate(4);
			buffer.order(ByteOrder.LITTLE_ENDIAN);
		}

		protected void received(Consumer.Peer peer)
		{
			log.debug("size received");

			Data data = new Data(buffer.getInt(0));
			peer.channel.read(data.buffer, peer, data);
		}
	}

	private class Data extends Receiver
	{
		private Data(int size)
		{
			buffer = ByteBuffer.allocate(size);
		}

		protected void received(Consumer.Peer peer)
		{
			log.debug("data received");

			buffer.limit(buffer.position());
			buffer.position(0);
			received(new Message(buffer));
		}
	}

	abstract void received(Message message);
}
