package com.ninchat.offhand;

public interface Commit extends AutoCloseable
{
	void engage();
	void cancel();
	void close();
}
