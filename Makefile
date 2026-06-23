.PHONY: build test fmt vet

build:
	go build ./...

test:
	go test ./... -count=1

fmt:
	@gofmt -s -l . | grep -q . && (gofmt -s -d .; exit 1) || true

vet:
	go vet ./...
