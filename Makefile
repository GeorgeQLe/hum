.PHONY: build run test clean

build:
	go build -o devctl .

run: build
	./devctl

test:
	go test -race ./...

clean:
	rm -f devctl
