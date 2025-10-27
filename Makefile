.PHONY: build build-optimized test bench clean

build:
	go build -o minivault ./src
	go build -ldflags="-s -w" -o minivault-optimized ./src

test:
	go test ./tests/...

bench:
	go test -bench=. -benchtime=2s ./tests/...

clean:
	rm -f minivault minivault-stripped
	rm -rf /tmp/mvtest-*

install:
	go build -ldflags="-s -w" -o minivault ./src
	cp minivault /usr/local/bin/

docker-build:
	docker build -t minivault:latest .

docker-run:
	docker-compose up -d
