PLUGIN_ID := com.github.officeutils.dm-export
VERSION := 0.1.0
PACKAGE := dist/$(PLUGIN_ID)-$(VERSION).tar.gz

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

package: build
	@rm -rf build/bundle
	@mkdir -p build/bundle/server/dist dist
	cp plugin.json build/bundle/plugin.json
	cp $(BINARIES) build/bundle/server/dist/
	tar -C build/bundle -czf $(PACKAGE) plugin.json server
	@printf 'Created %s\n' '$(PACKAGE)'

clean:
	rm -rf build dist server/dist
