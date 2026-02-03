.PHONY: build test clean install lint fmt

BINARY_NAME=provctl
MAIN_PATH=./cmd/provctl

build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

install: build
	sudo mv $(BINARY_NAME) /usr/local/bin/

test:
	go test -v -race -cover ./...

bench:
	go test -bench=. -benchmem ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f state.db

lint:
	golangci-lint run

fmt:
	gofmt -s -w .
	go mod tidy

run-aws:
	go run $(MAIN_PATH) create test-cluster --provider aws --region us-west-2

run-azure:
	go run $(MAIN_PATH) create test-cluster --provider azure --region eastus

list:
	go run $(MAIN_PATH) list
