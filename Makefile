VERSION ?= 0.1.0-dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY := claude-sync

.PHONY: build test test-integration clean install cross-compile

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/claude-sync

test:
	go test ./... -v

test-integration:
	go test ./tests/ -tags=integration -v

install: build
	cp $(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp $(BINARY) /usr/local/bin/$(BINARY)

cross-compile:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-darwin-arm64 ./cmd/claude-sync
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-darwin-amd64 ./cmd/claude-sync
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-linux-arm64 ./cmd/claude-sync
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-linux-amd64 ./cmd/claude-sync

clean:
	rm -f $(BINARY)
	rm -f plugin/bin/$(BINARY)-*
