version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-X main.version=" + version
bin     := "bin/grns"

build:
    go build -ldflags "{{ldflags}}" -o {{bin}} ./cmd/grns

test:
    go test ./...

test-integration: build
    bats tests/

test-smoke: build
    bats tests/cli_validation_errors.bats tests/cli_import_export.bats tests/cli_admin_cleanup.bats

lint:
    golangci-lint run ./...

clean:
    rm -rf bin/

install:
    go install -ldflags "{{ldflags}}" ./cmd/grns
