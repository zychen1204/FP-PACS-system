GOLANGCI_LINT ?= $(shell which golangci-lint 2>/dev/null || echo $(HOME)/bin/golangci-lint)

.PHONY: test lint vet

test:
	go test ./...

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run ./...
