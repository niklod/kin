.PHONY: build test test-e2e lint generate clean

BINARY := bin/kin
GO := go
GOFLAGS := -race

build:
	$(GO) build -o $(BINARY) ./cmd/kin

test:
	$(GO) test -race -cover ./...

test-e2e:
	$(GO) test -tags e2e -race -timeout 120s -v ./e2e/...

lint:
	golangci-lint run ./...

generate:
	cd proto && $(GO) generate

clean:
	rm -rf bin/ kinpb/*.pb.go

.DEFAULT_GOAL := build
