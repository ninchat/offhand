package com.ninchat.offhand;

import java.io.IOException;
import java.nio.channels.AsynchronousByteChannel;
import java.nio.channels.AsynchronousChannel;
import java.nio.channels.AsynchronousChannelGroup;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

public abstract class Consumer implements AutoCloseable
{
	private static final Log log = LogFactory.getLog(Consumer.class);

	protected abstract class Peer
	{
		protected AsynchronousByteChannel channel = null;
		private Message message;
		private Commit commit;

		private void reset()
		{
			message = null;
			commit = null;
		}

		protected void connected()
		{
			reset();

			log.debug("beginning");
			new Beginning().receive(this);
		}

		private class Beginning extends Command
		{
			boolean received(byte code)
			{
				switch (code) {
				case BEGIN:
					log.debug("payloading");
					new Payloading().receive(Peer.this);
					return true;

				default:
					return false;
				}
			}
		}

		private class Payloading extends Payload
		{
			void received(Message message)
			{
				assert Peer.this.message == null;

				if (message.valid()) {
					Peer.this.message = message;

					log.debug("acknowledgement");
					new Acknowledgement().send(Peer.this, Reply.RECEIVED);
				} else {
					log.error("corrupted message from " + Peer.this);
					disconnect();
				}
			}
		}

		private class Acknowledgement extends Reply
		{
			void sent()
			{
				log.debug("commencement");
				new Commencement().receive(Peer.this);
			}
		}

		private class Commencement extends Command
		{
			boolean received(byte code)
			{
				switch (code) {
				case COMMIT:
					assert message != null;
					Message message = Peer.this.message;
					Peer.this.message = null;

					assert commit == null;
					commit = new Commitment();

					log.debug("consumption");
					consume(message, commit);
					return true;

				case ROLLBACK:
					connected();
					return true;

				default:
					return false;
				}
			}
		}

		private class Commitment implements Commit
		{
			public void engage()
			{
				assert this == commit;
				commit = null;

				log.debug("engagement");
				new Conclusion().send(Peer.this, Reply.ENGAGED);
			}

			public void cancel()
			{
				assert this == commit;
				commit = null;

				log.debug("cancellation");
				new Conclusion().send(Peer.this, Reply.CANCELED);
			}

			public void close()
			{
				if (this == commit)
					cancel();
			}
		}

		private class Conclusion extends Reply
		{
			void sent()
			{
				connected();
			}
		}

		void disconnect()
		{
			close();
		}

		protected void close()
		{
			reset();

			if (channel != null) {
				AsynchronousChannel channel = this.channel;
				this.channel = null;

				try {
					channel.close();
				} catch (IOException e) {
					log.warn("peer closing", e);
				}
			}
		}

		public String toString()
		{
			return "peer";
		}
	}

	protected final AsynchronousChannelGroup channelGroup;

	public Consumer(AsynchronousChannelGroup channelGroup)
	{
		this.channelGroup = channelGroup;
	}

	protected abstract void consume(Message message, Commit commit);
}
