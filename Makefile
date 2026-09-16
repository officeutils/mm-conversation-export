PLUGIN_ID := com.github.officeutils.dm-export
PACKAGE_NAME := mm-dm-export
PACKAGE = dist/$(PACKAGE_NAME)-$(VERSION).tar.gz

BINARIES := \
	server/dist/plugin-linux-amd64 \
	server/dist/plugin-linux-arm64 \
	server/dist/plugin-darwin-amd64 \
	server/dist/plugin-darwin-arm64 \
	server/dist/plugin-windows-amd64.exe
SERVER_SOURCES := $(wildcard server/*.go) go.mod go.sum

.PHONY: all build clean package test

all: package

test:
	go test ./...

build: $(BINARIES)

$(BINARIES): $(SERVER_SOURCES)

server/dist/plugin-linux-amd64:
	@mkdir -p $(@D)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o $@ ./server

server/dist/plugin-linux-arm64:
	@mkdir -p $(@D)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o $@ ./server

server/dist/plugin-darwin-amd64:
	@mkdir -p $(@D)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o $@ ./server

server/dist/plugin-darwin-arm64:
	@mkdir -p $(@D)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o $@ ./server

server/dist/plugin-windows-amd64.exe:
	@mkdir -p $(@D)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o $@ ./server

package:
	@test -n "$(VERSION)" || { echo "VERSION is required (for example: make package VERSION=0.1.0)" >&2; exit 1; }
	@$(MAKE) build
	@set -eu; \
		mkdir -p build; \
		staging_dir=$$(mktemp -d build/package.XXXXXX); \
		trap 'rm -rf "$$staging_dir"' EXIT; \
		mkdir -p "$$staging_dir/server/dist" dist; \
		jq --arg version "$(VERSION)" '.version = $$version' plugin.json > "$$staging_dir/plugin.json"; \
		cp $(BINARIES) "$$staging_dir/server/dist/"; \
		rm -f dist/$(PACKAGE_NAME)-*.tar.gz; \
		tar -C "$$staging_dir" -czf "$(PACKAGE)" plugin.json server
	@printf 'Created %s\n' '$(PACKAGE)'

clean:
	rm -rf build dist server/dist
