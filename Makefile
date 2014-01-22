REDCARPET	:= redcarpet
MARKDOWN	:= $(REDCARPET) --parse-no_intra_emphasis --parse-autolink --parse-strikethrough --parse-fenced_code_blocks --parse-highlight --parse-tables

protocol.html: protocol.md
	( echo '<html><head>' && \
	  echo '<title>Offhand protocol</title>' && \
	  echo '<link href="https://github.global.ssl.fastly.net/assets/github-7155069f3ef445db5fc6503a1ff3eec8cf0a450d.css" type="text/css" rel="stylesheet" />' && \
	  echo '<link href="https://github.global.ssl.fastly.net/assets/github2-2ae3f4e67c9611aa6523df2bae070c14626217b6.css" type="text/css" rel="stylesheet" />' && \
	  echo '</head><body><div class="container"><article class="markdown-body">' && \
	  $(MARKDOWN) $^ && \
	  echo '</article></div></body></html>' \
	) > $@ || (rm -f $@; false)

clean::
	rm -f protocol.html
