.PHONY: build test clean install

build:
	go build -o unclaude ./cmd/unclaude

test:
	go test ./... -v

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

clean:
	rm -f unclaude coverage.out

install:
	go install

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet
	go test ./... -v
