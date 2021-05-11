GO		:= $(shell which go)
ROOT	:= $(shell pwd)

# CONFIG is the site configuration.
CONFIG		?= ''
# CONTENT is the contenent root
CONTENT		?= ''
# SITES is the root into which the site is generated
SITES 		?= ''
# TEMPLATES are golang templates used to render pages.
TEMPLATES	?= ''

gen:
	$(GO) run src/wwwgen/cmd/wwwgen/main.go -debug -config $(CONFIG) -content $(CONTENT) -sites $(SITES) -templates $(TEMPLATES)
