APP        := journey-service
MODULE     := github.com/kpkipper/journey-service
BINARY     := bin/$(APP)
PROJECT_ID := jourlytrip
REGION     := asia-southeast1
IMAGE      := $(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(APP)/$(APP)

.PHONY: run build test wire swag.gen mock.gen ci.lint docker.build docker.push deploy.run deploy.hosting deploy

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

docker.build:
	gcloud builds submit --tag $(IMAGE)

deploy.run:
	gcloud run deploy $(APP) \
		--image $(IMAGE) \
		--region $(REGION) \
		--platform managed \
		--allow-unauthenticated \
		--set-env-vars "APP_PORT=8080,DB_DSN=$(DB_DSN)"

deploy.hosting:
	firebase deploy --only hosting

deploy: docker.build deploy.run deploy.hosting
