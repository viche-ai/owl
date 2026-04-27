#!/usr/bin/env bash
set -euo pipefail

fail=0

check() {
  local pattern="$1"
  local message="$2"

  if rg -n --glob '*_test.go' "$pattern" .; then
    echo "FAIL: $message"
    fail=1
  fi
}

check 'time\.Sleep\(' "Tests must not sleep; use context deadlines or channel synchronization"
check 'os\.UserHomeDir\(' "Tests must not touch the real home dir; use t.TempDir() or t.Setenv(\"HOME\", ...)"
check '(?:"|'"'"')~\/\.owl|filepath\.Join\([^)]*home[^)]*,\s*"\.owl"' "Tests must not reference the real ~/.owl path"
check '(^|[^[:alnum:]_])http\.NewRequest\(' "Use httptest.NewRequest in tests, not http.NewRequest"

exit "$fail"
