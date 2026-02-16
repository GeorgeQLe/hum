.PHONY: build build-envsafe build-web run dev test lint clean install install-skill

build:
	go build -o devctl .

build-web:
	cd web && npm install && npm run build
	mkdir -p internal/server/web_dist
	cp -r web/dist/* internal/server/web_dist/

build-envsafe: build-web
	go build -o envsafe ./cmd/envsafe

run: build
	./devctl

dev: build
	./devctl dev

test:
	go test -race ./...

lint:
	golangci-lint run ./...

clean:
	rm -f devctl envsafe

install:
	bash install.sh

install-skill:
	bash install-skill.sh
