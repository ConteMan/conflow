WEB_DIR := web

.PHONY: bootstrap web-install web-dev web-build web-check api-generate api-check docs-check fmt test vet build check

bootstrap:
	go mod download
	npm --prefix $(WEB_DIR) install
	$(MAKE) web-build

web-install:
	npm --prefix $(WEB_DIR) ci

web-dev:
	npm --prefix $(WEB_DIR) run dev

web-build:
	npm --prefix $(WEB_DIR) run build
	go run ./cmd/embedui

web-check:
	npm --prefix $(WEB_DIR) run build
	go run ./cmd/embedui
	git diff --exit-code -- internal/webui/assets

api-generate:
	npm --prefix $(WEB_DIR) run api:generate

api-check:
	$(MAKE) api-generate
	git diff --exit-code -- web/src/api/schema.d.ts

docs-check:
	go run ./cmd/checkdocs

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './web/*')

test:
	go test ./...

vet:
	go vet ./...

build:
	mkdir -p bin
	go build -o bin/conflow ./cmd/conflow

check:
	$(MAKE) api-check
	$(MAKE) web-check
	$(MAKE) docs-check
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './web/*'))"
	$(MAKE) test
	$(MAKE) vet
	$(MAKE) build
