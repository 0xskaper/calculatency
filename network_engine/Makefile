.PHONY: build run test clean docker-build docker-run

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=network-engine

all: build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: build
	sudo ./$(BINARY_NAME)

run-unprivileged: build
	@echo "Warning: Running without root privileges. Some features may not work."
	./$(BINARY_NAME)

deps:
	$(GOGET) github.com/gin-gonic/gin
	$(GOGET) golang.org/x/net/icmp
	$(GOGET) golang.org/x/net/ipv4

# Docker commands
docker-build:
	docker build -t network-engine .

docker-run:
	docker run --rm -p 8080:8080 --cap-add=NET_RAW network-engine

docker-run-privileged:
	docker run --rm -p 8080:8080 --privileged network-engine

# Development
dev: build
	sudo ./$(BINARY_NAME)

# Install with proper capabilities (Linux only)
install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/
	sudo setcap cap_net_raw+ep /usr/local/bin/$(BINARY_NAME)
