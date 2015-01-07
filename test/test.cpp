#include <cassert>
#include <cerrno>
#include <cstddef>
#include <cstdint>
#include <cstdio>
#include <cstring>

#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/types.h>

#include <netinet/in.h>

#include <linux/sockios.h>

#include <netdb.h>
#include <unistd.h>

#define WINDOW_SIZE  65536

static const struct addrinfo hints = {
	/* ai_flags    */ AI_ADDRCONFIG,
	/* ai_family   */ AF_UNSPEC,
	/* ai_socktype */ SOCK_STREAM,
};

class Link
{
public:
	Link(const char *hostname, const char *service):
		m_hostname(hostname),
		m_service(service),
		m_fd(-1),
		m_sent(0)
	{
	}

	~Link()
	{
		close();
	}

	ssize_t write(const char *buf, size_t size)
	{
		while (true) {
			ssize_t written;

			if (m_fd < 0)
				connect();

			written = 0;

			while (written < size) {
				ssize_t n;

				n = ::write(m_fd, buf + written, size - written);

				if (n < 0 && errno == EINTR)
					continue;

				if (n <= 0) {
					do_the_thing_you_do(buf, written);
					buf += written;
					size -= written;
					break;
				}

				written += n;
			}

			if (m_fd >= 0)
				return written;
		}
	}

private:
	Link(const Link &);
	void operator=(const Link &);

	void connect()
	{
		while (true) {
			struct addrinfo *info;
			int fd;
			ssize_t sent;
			int value;
			uint64_t pos;
			const char *buf;
			ssize_t written;
			ssize_t size;

			if (getaddrinfo(m_hostname, m_service, &hints, &info) != 0)
				continue;

			fd = -1;

			for (struct addrinfo *i = info; i; i = i->ai_next) {
				fd = socket(i->ai_family, i->ai_socktype, i->ai_protocol);
				if (fd < 0)
					continue;

				if (::connect(fd, i->ai_addr, i->ai_addrlen) < 0) {
					close(fd);
					fd = -1;
					continue;
				}

				break;
			}

			freeaddrinfo(info);

			if (fd < 0)
				continue;

			pos = m_sent & ~(WINDOW_SIZE - 1);
			buf = reinterpret_cast<char *> (&pos);
			written = 0;

			do {
				ssize_t n;

				n = ::write(fd, buf + written, sizeof (uint64_t) - written);

				if (n < 0 && errno == EINTR)
					continue;

				if (n <= 0) {
					::close(fd);
					fd = -1;
					break;
				}

				written += n;
			} while (written < sizeof (uint64_t));

			if (fd < 0)
				continue;

			size = m_sent - pos;
			written = 0;

			if (size > 0) {
				do {
					ssize_t n;

					n = ::write(fd, m_window + written, size - written);

					if (n < 0 && errno == EINTR)
						continue;

					if (n <= 0) {
						::close(fd);
						fd = -1;
						break;
					}

					written += n;
				} while (written < sizeof (uint64_t));
			}

			m_fd = fd;
			break;
		}
	}

	void close()
	{
		if (m_fd >= 0)
			::close(m_fd);
	}

	void do_the_thing_you_do(const char *buf, ssize_t written)
	{
		// TODO
	}

	int m_fd;
	uint64_t m_sent;
	char m_window[WINDOW_SIZE];
};

#define HOSTNAME  "example.net"
#define SERVICE   "80"

static const char *data = "GET / HTTP/1.0\r\n\r\n";
static const ssize_t datalen = strlen(data);

int main()
{
	struct addrinfo hints;
	struct addrinfo *info;
	int error;
	int fd;
	ssize_t sent;
	int value;

	memset(&hints, 0, sizeof (hints));
	hints.ai_flags = AI_ADDRCONFIG;
	hints.ai_family = AF_UNSPEC;
	hints.ai_socktype = SOCK_STREAM;

	error = getaddrinfo(HOSTNAME, SERVICE, &hints, &info);
	if (error) {
		fprintf(stderr, "getaddrinfo: %s\n", gai_strerror(error));
		return 1;
	}

	fd = -1;

	for (struct addrinfo *i = info; i; i = i->ai_next) {
		fd = socket(i->ai_family, i->ai_socktype, i->ai_protocol);
		if (fd < 0)
			continue;

		if (connect(fd, i->ai_addr, i->ai_addrlen) < 0) {
			int save = errno;
			close(fd);
			fd = -1;
			errno = save;
			continue;
		}

		break;
	}

	error = errno;

	freeaddrinfo(info);

	if (fd < 0) {
		errno = error;
		perror(HOSTNAME);
		return 1;
	}

	sent = 0;

	do {
		ssize_t n;

		n = write(fd, data + sent, datalen - sent);
		if (n < 0 && errno != EINTR) {
			perror("write");
			return 1;
		}

		if (n > 0)
			sent += n;
	} while (sent < datalen);

	do {
		if (ioctl(fd, SIOCOUTQ, &value) < 0) {
			perror("ioctl");
			return 1;
		}

		printf("%d\n", value);
	} while (value > 0);

	close(fd);

	return 0;
}
