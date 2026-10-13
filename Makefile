.PHONY: swagger generate test lint fmt run

swagger:
	swag init -g internal/transport/http/server.go

generate:
	go generate ./...

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

run:
	go run cmd/server/main.go
