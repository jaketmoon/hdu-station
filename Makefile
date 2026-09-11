GO ?= go

.PHONY: test frontend-check frontend-install build e2e live-test

frontend-install:
	cd frontend && npm ci

frontend-check:
	cd frontend && npm run typecheck
	cd frontend && npm run build

test: frontend-check
	$(GO) test ./...
	cd frontend && npm test

build: frontend-check
	mkdir -p build/bin
	$(GO) build -tags desktop,production,wv2runtime.download -o build/bin/hdu-station .
	./scripts/package-local.sh

e2e:
	cd frontend && npx playwright test

live-test:
	HDU_STATION_LIVE_TEST=1 $(GO) test ./internal/agent -run TestLiveAcceptance -v -count=1 -timeout=20m
