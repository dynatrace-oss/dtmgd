BINARY   := dtmgd
VERSION  := $(shell git describe --tags --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-s -w \
	-X github.com/dynatrace-oss/dtmgd/pkg/version.Version=$(VERSION) \
	-X github.com/dynatrace-oss/dtmgd/pkg/version.Commit=$(COMMIT) \
	-X github.com/dynatrace-oss/dtmgd/pkg/version.Date=$(DATE)"

.PHONY: build clean install vet fmt test lint release-dry

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

lint: vet fmt
	@UNFORMATTED=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED" ]; then echo "Unformatted files: $$UNFORMATTED"; exit 1; fi

release-dry:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY)
