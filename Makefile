CGO_ENABLED=0
export CGO_ENABLED

.PHONY: all server importer clean run import import-no-tmdb release snapshot

all: server importer

server:
	go build -o bin/server ./cmd/server/

importer:
	go build -o bin/importer ./cmd/importer/

run: server
	./bin/server

import: importer
	./bin/importer -data ./data -db ./watchlog.db

import-no-tmdb: importer
	./bin/importer -data ./data -db ./watchlog.db -skip-tmdb

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin/ dist/ watchlog.db watchlog.db-wal watchlog.db-shm
