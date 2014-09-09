REDCARPET	:= redcarpet
MARKDOWN	:= $(REDCARPET) --parse-no_intra_emphasis --parse-autolink --parse-strikethrough --parse-fenced_code_blocks --parse-highlight --parse-tables

all: protocol.html relish

protocol.html: protocol.md Makefile
	( echo '<!doctype html><html><head><meta charset="UTF-8">' && \
	  echo '<title>Relink Protocol</title>' && \
	  echo '<link href="https://assets-cdn.github.com/assets/github-999cec161edd06579979a1b0d70e9ce1f4473328.css" media="all" type="text/css" rel="stylesheet"/>' && \
	  echo '<link href="https://assets-cdn.github.com/assets/github2-629d0ad48f9b722f5faa18674040274fd4e7c247.css" media="all" type="text/css" rel="stylesheet"/>' && \
	  echo '</head><body><div class="container"><article class="markdown-body">' && \
	  $(MARKDOWN) protocol.md && \
	  echo '</article></div></body></html>' \
	) > $@ || (rm -f $@; false)

relish::
	$(MAKE) -C go ../relish

clean::
	rm -f protocol.html resh
