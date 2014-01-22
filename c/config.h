#ifndef TAP_PORTABLE_BYTEORDER
# ifdef __linux__
#  include <endian.h>
# else
#  include <sys/types.h>
# endif
# if defined(BYTE_ORDER)
#  if BYTE_ORDER == LITTLE_ENDIAN
#   define HAVE_LITTLE_ENDIAN   1
#   define HAVE_BIG_ENDIAN      0
#  else
#   define HAVE_LITTLE_ENDIAN   0
#   define HAVE_BIG_ENDIAN      1
#  endif
# elif defined(__BYTE_ORDER)
#  if __BYTE_ORDER == __LITTLE_ENDIAN
#   define HAVE_LITTLE_ENDIAN   1
#   define HAVE_BIG_ENDIAN      0
#  else
#   define HAVE_LITTLE_ENDIAN   0
#   define HAVE_BIG_ENDIAN      1
#  endif
# elif defined(__APPLE__)
#  if defined(__LITTLE_ENDIAN__)
#   define HAVE_LITTLE_ENDIAN   1
#   define HAVE_BIG_ENDIAN      0
#  else
#   define HAVE_LITTLE_ENDIAN   0
#   define HAVE_BIG_ENDIAN      1
#  endif
# endif
#endif
