.PHONY: fmt test vet race check build

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

check: fmt vet test race

build:
	go build -o costroid-sync .
