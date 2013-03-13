package test;

import java.nio.ByteBuffer;
import java.nio.channels.AsynchronousChannelGroup;
import java.nio.charset.Charset;
import java.util.Random;

import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

import com.ninchat.offhand.Commit;
import com.ninchat.offhand.ConnectingConsumer;
import com.ninchat.offhand.Message;

public class TestConsumer extends ConnectingConsumer
{
	private static final Log log = LogFactory.getLog(TestConsumer.class);

	private Random random = new Random();
	private Charset charset = Charset.forName("UTF-8");

	public TestConsumer(AsynchronousChannelGroup channelGroup)
	{
		super(channelGroup);
	}

	protected void consume(Message message, Commit commit)
	{
		try {
			if (random.nextBoolean()) {
				commit.engage();
			} else {
				return;
			}
		} finally {
			commit.close();
		}

		// print only the second part
		ByteBuffer part = message.get(1);
		byte[] array = part.array();
		int offset = part.arrayOffset() + part.position();
		int length = part.limit() - part.position();
		log.info(new String(array, offset, length, charset));
	}
}
