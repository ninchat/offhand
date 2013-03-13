package com.ninchat.offhand;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.ArrayList;
import java.util.List;

public class Message
{
	private List<ByteBuffer> parts;

	Message(ByteBuffer data)
	{
		data.order(ByteOrder.LITTLE_ENDIAN);

		List<ByteBuffer> parts = new ArrayList<ByteBuffer>();

		for (int offset = 0; true; ) {
			int remain = data.limit() - offset;
			if (remain == 0)
				break;
			if (remain < 4)
				return;

			int size = data.getInt(offset);
			offset += 4;

			// size vs. limit will be checked automatically on the next round

			ByteBuffer part = data.slice();
			part.position(offset);
			part.limit(offset + size);
			parts.add(part);

			offset += size;
		}

		this.parts = parts;
	}

	boolean valid()
	{
		return parts != null;
	}

	public int length()
	{
		return parts.size();
	}

	public ByteBuffer get(int index)
	{
		return parts.get(index);
	}
}
