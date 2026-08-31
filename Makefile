.PHONY: build test bench coverage clean install fmt vet lint

build:
	go build -o unclaude ./cmd/unclaude

test:
	go test ./... -race -count=1

bench:
	go test ./internal/cleaner -bench=. -benchmem -run=^$$

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

clean:
	rm -f unclaude coverage.out

install:
	go install ./cmd/unclaude

fmt:
	gofmt -w .

vet:
	go vet ./...

lint: fmt vet
	go test ./... -race -count=1
