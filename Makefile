GO		:= $(shell which go)
ROOT	:= $(shell pwd)

# SRC is the root directory of the source files
SRC	?= '../www-src'
# DST is the root into which the sites are generated
DST	?= '../www'
# GENOTPS are additional flags passed to wwwgen.
# MONOPTS are additional flags passed to wwwmon.
MONOPTS ?= ''

.PHONY: dep gen mon

src/vendor/github.com/rusross/blackfriday/.git:
	$(call git_clone,git@github.com:russross/blackfriday.git,src/vendor/github.com/russross/blackfriday,4c9bf9512682b995722660a4196c0013228e2049)

dep: src/vendor/github.com/russross/blackfriday/.git

gen: dep
	GOPATH=$(ROOT) $(GO) run src/cmd/wwwgen/main.go -config $(SRC)/sites.json \
		-content $(SRC)/content -monitor bin/monitor.json -sites $(DST) \
		-templates $(SRC)/templates$(GENOPTS)

mon:
	GOPATH=$(ROOT) $(GO) run src/cmd/wwwmon/main.go -config bin/monitor.json$(MONOPTS)

define git_clone
	@echo "$(3): $(2) < $(1)"
	@git clone $1 $2 1>/dev/null 2>&1
	@git -C $2 checkout $3 1>/dev/null 2>&1
endef
