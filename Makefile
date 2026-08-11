.DEFAULT_GOAL := help

.PHONY: help hz gen-dal build test check

help:
	@echo "make hz       Generate Hertz routes/models/OpenAPI"
	@echo "make gen-dal  Generate typed database queries"
	@echo "make build    Build the Go backend"
	@echo "make test     Run backend tests"
	@echo "make check    Run backend and frontend checks"

hz:
	@test -f idl/api.thrift || (echo "Create idl/api.thrift before running make hz" && exit 1)
	hz update -idl idl/api.thrift -enable_extends --customize_package=template/package.yaml -thrift-plugins=http-swagger

gen-dal:
	gorm gen -i ./types/dbmodel -o ./types/dbmodel/generated

build:
	go build ./...

test:
	go test ./...

check: test build
	cd frontend/app && pnpm run check
	cd frontend/admin && pnpm run build
