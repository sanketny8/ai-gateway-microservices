.PHONY: all build run test lint fmt clean docker-build docker-run

all: build

build:
	go build -o bin/gateway main.go

run:
	go run main.go

test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

lint:
	golangci-lint run

fmt:
	go fmt ./...
	gofmt -s -w .

vet:
	go vet ./...

clean:
	rm -rf bin/
	rm -f coverage.out

docker-build:
	docker build -t ai-gateway:latest .

docker-run:
	docker run -p 8080:8080 ai-gateway:latest

deps:
	go mod download
	go mod tidy

