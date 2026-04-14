APP     := journey-service
MODULE  := github.com/kpkipper/journey-service
BINARY  := bin/$(APP)

.PHONY: run build test wire swag.gen mock.gen ci.lint

run:
	go run main.go

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY) main.go

test:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out

wire:
	wire ./...

swag.gen:
	swag init -g main.go --output docs

mock.gen:
	mockery --all --keeptree --output internal/mocks

ci.lint:
	golangci-lint run --fix ./...
