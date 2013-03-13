package test;

import java.net.InetSocketAddress;
import java.nio.channels.AsynchronousChannelGroup;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

public class Main
{
	public static void main(String[] args)
	{
		try {
			ExecutorService executor = Executors.newSingleThreadExecutor();
			AsynchronousChannelGroup channelGroup = AsynchronousChannelGroup.withThreadPool(executor);

			TestConsumer consumer = new TestConsumer(channelGroup);
			consumer.connect(new InetSocketAddress("localhost", 9000));

			while (!channelGroup.awaitTermination(5, TimeUnit.SECONDS))
				consumer.reconnect();

		} catch (Throwable e) {
			e.printStackTrace(System.err);
		}
	}
}
