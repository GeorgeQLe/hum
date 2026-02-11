.PHONY: build run test clean install

build:
	go build -o devctl .

run: build
	./devctl

test:
	go test -race ./...

clean:
	rm -f devctl

install:
	bash install.sh
