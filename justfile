version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-X main.version=" + version
bin     := "bin/grns"

build:
    go build -ldflags "{{ldflags}}" -o {{bin}} ./cmd/grns

test:
    go test ./...

test-integration: build
    bats tests/

lint:
    golangci-lint run ./...

clean:
    rm -rf bin/

install:
    go install -ldflags "{{ldflags}}" ./cmd/grns
