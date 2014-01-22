#ifndef OFFHAND_ATOMIC_H
#define OFFHAND_ATOMIC_H

static inline void atomic_inc(unsigned long *p)
{
	__sync_fetch_and_add(p, 1);
}

static inline unsigned long atomic_dec(unsigned long *p)
{
	return __sync_fetch_and_sub(p, 1);
}

#endif
