WEB_DIR := web

.PHONY: bootstrap web-install web-dev web-build web-check fmt test vet build check

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
	$(MAKE) web-check
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './web/*'))"
	$(MAKE) test
	$(MAKE) vet
	$(MAKE) build
