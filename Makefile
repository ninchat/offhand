REDCARPET	:= redcarpet
MARKDOWN	:= $(REDCARPET) --parse-no_intra_emphasis --parse-autolink --parse-strikethrough --parse-fenced_code_blocks --parse-highlight --parse-tables

all: protocol.html shell

protocol.html: protocol.md Makefile
	( echo '<!doctype html><html><head><meta charset="UTF-8">' && \
	  echo '<title>Offhand Protocol</title>' && \
	  echo '<link href="https://assets-cdn.github.com/assets/github-da5200f93aaeb15e21a7b6b64385a334ba72ccb61687685e7e02dfc0b6a2ec1d.css" media="all" type="text/css" rel="stylesheet"/>' && \
	  echo '<link href="https://assets-cdn.github.com/assets/github2-7db4c7ec8e8941c3b226bc8a9ec014bbe1ec0a320cbdfcb7600d7f6a4a184b37.css" media="all" type="text/css" rel="stylesheet"/>' && \
	  echo '</head><body><div class="container"><article class="markdown-body">' && \
	  $(MARKDOWN) protocol.md && \
	  echo '</article></div></body></html>' \
	) > $@ || (rm -f $@; false)

shell::
	$(MAKE) -C go ../shell

clean::
	rm -f protocol.html shell
