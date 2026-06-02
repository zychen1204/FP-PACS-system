GOLANGCI_LINT ?= $(shell which golangci-lint 2>/dev/null || echo $(HOME)/bin/golangci-lint)

.PHONY: test test-race lint vet

test:
	go test ./...

test-race:
	go test -race -timeout=180s ./...

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run ./...
