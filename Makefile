.PHONY: build build-envsafe run test clean install install-skill

build:
	go build -o devctl .

build-envsafe:
	go build -o envsafe ./cmd/envsafe

run: build
	./devctl

test:
	go test -race ./...

clean:
	rm -f devctl envsafe

install:
	bash install.sh

install-skill:
	bash install-skill.sh
