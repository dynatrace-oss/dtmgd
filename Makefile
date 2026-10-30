BINARY   := dtmgd
VERSION  := $(shell git describe --tags --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-s -w \
	-X github.com/dynatrace-oss/dtmgd/pkg/version.Version=$(VERSION) \
	-X github.com/dynatrace-oss/dtmgd/pkg/version.Commit=$(COMMIT) \
	-X github.com/dynatrace-oss/dtmgd/pkg/version.Date=$(DATE)"

.PHONY: build clean install vet fmt test lint vulncheck release-dry

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) .

install:
	go install $(LDFLAGS) .

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Keep in step with .github/workflows/sast.yml.
GOLANGCI_LINT_VERSION := v2.15.0

# Checks only; make fmt rewrites. actionlint is pinned and checksum-locked in
# tools/go.mod and go.sum.
lint: vet
	@UNFORMATTED=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED" ]; then echo "Unformatted files (run make fmt):"; echo "$$UNFORMATTED"; exit 1; fi
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: install $(GOLANGCI_LINT_VERSION) (https://golangci-lint.run/docs/welcome/install/)"; exit 1; }
	golangci-lint run
	go -C tools build -o "$(CURDIR)/bin/actionlint" github.com/rhysd/actionlint/cmd/actionlint
	./bin/actionlint

# govulncheck is pinned and checksum-locked in tools/go.mod and go.sum.
vulncheck:
	go -C tools build -o "$(CURDIR)/bin/govulncheck" golang.org/x/vuln/cmd/govulncheck
	./bin/govulncheck ./...

release-dry:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY)
	rm -rf bin
