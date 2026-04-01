VERSION ?= 0.11.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY := claude-sync

.PHONY: build test test-integration clean install cross-compile copy-plugin smoke-hooks smoke-hooks-live

copy-plugin:
	rm -rf internal/bundled/plugin
	cp -r plugin internal/bundled/plugin
	rm -rf internal/bundled/plugin/bin

build: copy-plugin
	go build $(LDFLAGS) -o $(BINARY) ./cmd/claude-sync

test: copy-plugin
	go test ./... -v

test-integration: copy-plugin
	go test ./tests/ -tags=integration -v

install: build
	@INSTALL_DIR=""; \
	if [ -n "$(GOPATH)" ] && echo "$$PATH" | tr ':' '\n' | grep -q "$(GOPATH)/bin"; then \
		INSTALL_DIR="$(GOPATH)/bin"; \
	elif echo "$$PATH" | tr ':' '\n' | grep -q "$$HOME/.local/bin"; then \
		INSTALL_DIR="$$HOME/.local/bin"; \
	elif [ -w /usr/local/bin ]; then \
		INSTALL_DIR="/usr/local/bin"; \
	else \
		INSTALL_DIR="$$HOME/.local/bin"; \
	fi; \
	mkdir -p "$$INSTALL_DIR"; \
	cp $(BINARY) "$$INSTALL_DIR/$(BINARY)"; \
	echo "Installed $(BINARY) to $$INSTALL_DIR/$(BINARY)"; \
	if ! command -v $(BINARY) >/dev/null 2>&1; then \
		echo ""; \
		echo "NOTE: $$INSTALL_DIR is not in your PATH."; \
		echo "Add this to your shell config:"; \
		echo "  export PATH=\"$$INSTALL_DIR:\$$PATH\""; \
	fi

cross-compile: copy-plugin
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-darwin-arm64 ./cmd/claude-sync
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-darwin-amd64 ./cmd/claude-sync
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-linux-arm64 ./cmd/claude-sync
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-linux-amd64 ./cmd/claude-sync

smoke-hooks: build
	bash scripts/smoke-test-hooks.sh

smoke-hooks-live: build
	bash scripts/smoke-test-hooks.sh --live

clean:
	rm -f $(BINARY)
	rm -f plugin/bin/$(BINARY)-*
