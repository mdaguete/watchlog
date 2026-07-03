CGO_ENABLED=0
export CGO_ENABLED

.PHONY: all server clean run release snapshot

all: server

server:
	go build -o bin/server ./cmd/server/

run: server
	./bin/server

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin/ dist/ watchlog.db watchlog.db-wal watchlog.db-shm
