.PHONY: build test lint generate clean

BINARY := bin/kin
GO := go
GOFLAGS := -race

build:
	$(GO) build -o $(BINARY) ./cmd/kin

test:
	$(GO) test -race -cover ./...

lint:
	golangci-lint run ./...

generate:
	cd proto && $(GO) generate

clean:
	rm -rf bin/ kinpb/*.pb.go

.DEFAULT_GOAL := build
