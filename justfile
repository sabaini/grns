version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-X main.version=" + version
bin     := "bin/grns"

build:
    go build -ldflags "{{ldflags}}" -o {{bin}} ./cmd/grns

snap:
    snapcraft pack -v

test:
    go test ./...

tests-all:
    just tidy-check
    just fmt-check
    just test-contracts
    just test
    just test-coverage-critical
    just test-integration
    just test-py
    just test-py-hypothesis

test-contracts:
    go test ./cmd/grns -run '^TestReadme' -count=1

tidy-check:
    go mod tidy -diff

fmt:
    gofmt -w $(find cmd internal -name '*.go')

fmt-check:
    test -z "$(gofmt -l $(find cmd internal -name '*.go'))"

test-integration: build
    bats tests/

test-smoke: build
    bats tests/cli_autospawn.bats tests/cli_create_show.bats tests/cli_admin_cleanup.bats

test-py: build
    uv run python3 -m pytest -q tests_py

test-py-concurrency: build
    python3 -m pytest -q tests_py/test_concurrency.py tests_py/test_concurrency_stress.py

test-py-stress: build
    GRNS_STRESS=1 python3 -m pytest -q -s -m stress tests_py/test_stress_mixed_workload.py

compare-stress baseline candidate:
    python3 tests/ci/compare_stress_summaries.py {{baseline}} {{candidate}}

test-py-hypothesis: build
    uv run python3 -m pytest -q -m hypothesis tests_py

test-py-perf: build
    GRNS_PYTEST_PERF=1 uv run python3 -m pytest -q -m perf tests_py

bench-go-perf:
    go test ./internal/server -run '^$' -bench 'BenchmarkTaskService' -benchmem -count=1

test-go-perf:
    GRNS_PERF_ENFORCE=1 go test ./internal/server -run '^TestPerformanceBudgets$' -count=1 -v

test-coverage-critical:
    ./tests/ci/check_critical_coverage.sh

lint:
    golangci-lint run ./...

clean:
    rm -rf bin/

install:
    go install -ldflags "{{ldflags}}" ./cmd/grns
