GO ?= go
WAILS_VERSION ?= v2.14.0

.PHONY: test frontend-check frontend-install build package-darwin

test:
	$(GO) test ./...
	cd frontend && npm test

frontend-check:
	cd frontend && npm run typecheck
	cd frontend && npm run build

frontend-install:
	cd frontend && npm install

build: frontend-check
	mkdir -p build/bin
	$(GO) build -o build/bin/hdu-station ./...

package-darwin:
	GO=$(GO) WAILS_VERSION=$(WAILS_VERSION) ./scripts/package-darwin.sh
