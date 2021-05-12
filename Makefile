GO		:= $(shell which go)
ROOT	:= $(shell pwd)

# SRC is the root directory of the source files
SRC	?= ''
# DST is the root into which the sites are generated
DST	?= ''

.PHONY: gen mon

gen:
	$(GO) run src/wwwgen/cmd/wwwgen/main.go -debug -config $(SRC)/sites.json -content $(SRC)/content -monitor bin/monitor.json -sites $(DST) -templates $(SRC)/templates

mon:
	$(GO) run src/wwwmon/cmd/wwwmon/main.go -debug -config bin/monitor.json
