GO ?= go
WAILS_VERSION ?= v2.14.0

.PHONY: test frontend-check frontend-install build sandbox-runtime sandbox-image-metadata package-darwin sandbox-image-darwin-arm64

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
	$(GO) build -o build/bin/hdu-station .

sandbox-runtime:
	mkdir -p build/bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -o build/bin/hdu-station-runtime-linux-amd64 ./cmd/sandbox-runtime
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -o build/bin/hdu-station-runtime-linux-arm64 ./cmd/sandbox-runtime

sandbox-image-metadata:
	mkdir -p build/bin
	$(GO) build -trimpath -o build/bin/hdu-station-sandbox-image-metadata ./cmd/sandbox-image-metadata

package-darwin:
	GO=$(GO) WAILS_VERSION=$(WAILS_VERSION) ./scripts/package-darwin.sh

sandbox-image-darwin-arm64:
	./scripts/build-sandbox-image-darwin-arm64.sh
