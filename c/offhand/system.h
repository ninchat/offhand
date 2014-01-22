#ifndef OFFHAND_SYSTEM_H
#define OFFHAND_SYSTEM_H

#include <sys/socket.h>
#include <sys/types.h>

#ifdef __linux__
static inline int system_socket(int family, int type, int protocol)
{
	return socket(family, type | SOCK_NONBLOCK | SOCK_CLOEXEC, protocol);
}

static inline int system_accept(int sockfd, struct sockaddr *addr, socklen_t *addrlen)
{
	return accept4(sockfd, addr, addrlen, SOCK_NONBLOCK | SOCK_CLOEXEC);
}
#else
# error TODO: non-Linux system_socket() and system_accept()
#endif

#endif
