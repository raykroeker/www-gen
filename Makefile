GO		:= $(shell which go)
ROOT	:= $(shell pwd)

# SRC is the root directory of the source files
SRC	?= '../www-src'
# DST is the root into which the sites are generated
DST	?= '../www'
# GENOTPS are additional flags passed to wwwgen.
# MONOPTS are additional flags passed to wwwmon.
MONOPTS ?= ''

.PHONY: gen mon

gen:
	$(GO) run src/cmd/wwwgen/main.go -config $(SRC)/sites.json \
		-content $(SRC)/content -monitor bin/monitor.json -sites $(DST) \
		-templates $(SRC)/templates$(GENOPTS)

mon:
	$(GO) run src/cmd/wwwmon/main.go -config bin/monitor.json$(MONOPTS)
