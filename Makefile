.PHONY: build test docker migrate seed run lint clean vet

BINARY=nest

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/server/

test:
	go test ./... -v -count=1

vet:
	go vet ./...

lint:
	go vet ./...

docker:
	docker build -t nest:latest .

migrate:
	go run ./cmd/migrate/

seed:
	go run ./cmd/seed/

run:
	go run ./cmd/server/

clean:
	rm -f $(BINARY)
