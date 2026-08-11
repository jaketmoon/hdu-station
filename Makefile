GOCACHE ?= /tmp/hdu-station-gocache
GOMODCACHE ?= /tmp/hdu-station-gomodcache
GOPATH ?= /tmp/hdu-station-gopath

.PHONY: test frontend-check frontend-install build

test:
	env GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) go test ./...
	cd frontend && npm test

frontend-check:
	cd frontend && npm run typecheck
	cd frontend && npm run build

frontend-install:
	cd frontend && npm install

build: frontend-check
	mkdir -p build/bin
	env GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) go build -o build/bin/hdu-station ./...
