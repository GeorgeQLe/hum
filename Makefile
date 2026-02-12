.PHONY: build run test clean install install-skill

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

install-skill:
	bash install-skill.sh
