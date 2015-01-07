CFLAGS		+= -g -Wall -Wextra
CXXFLAGS	+= -std=c++11

SOURCES		:= test.cpp

test: $(SOURCES)
	$(CXX) $(CPPFLAGS) $(CXXFLAGS) $(CFLAGS) -o $@ $(SOURCES) $(LDFLAGS) $(LIBS)

clean::
	rm -f test
