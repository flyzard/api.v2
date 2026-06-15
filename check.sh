#!/usr/bin/env bash
# Repo health gate: a green run approximates "compliant and reliable".
# Required: gofmt, go vet, go test. Optional (skipped with a note if absent):
# staticcheck, govulncheck.
set -uo pipefail
cd "$(dirname "$0")"

fail=0

unformatted=$(gofmt -l ./cmd ./internal 2>/dev/null)
if [ -n "$unformatted" ]; then
	echo "FAIL gofmt — needs formatting:"
	echo "$unformatted"
	fail=1
fi

echo "== go vet"
go vet ./... || fail=1

if command -v staticcheck >/dev/null 2>&1; then
	echo "== staticcheck"
	staticcheck ./... || fail=1
else
	echo "SKIP staticcheck (go install honnef.co/go/tools/cmd/staticcheck@latest)"
fi

if command -v govulncheck >/dev/null 2>&1; then
	echo "== govulncheck"
	govulncheck ./... || fail=1
else
	echo "SKIP govulncheck (go install golang.org/x/vuln/cmd/govulncheck@latest)"
fi

echo "== go test"
go test ./... || fail=1

if [ "$fail" -ne 0 ]; then
	echo "CHECK FAILED"
	exit 1
fi
echo "ALL CHECKS PASSED"
