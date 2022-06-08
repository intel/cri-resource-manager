# Utilities to verify data from metrics

verify_metrics_url="http://localhost:8891/metrics"

verify-metrics-has-line() {
    local expected_line="$1"
    vm-run-until --timeout 10 "echo 'waiting for metrics line: $expected_line' >&2; curl --silent $verify_metrics_url | grep -E '$expected_line'" || {
        command-error "expected line '$1' missing from the output"
    }
}

verify-metrics-has-no-line() {
    local unexpected_line="$1"
    vm-command "echo 'checking absense of metrics line: $unexpected_line' >&2; curl --silent $verify_metrics_url | grep -E '$unexpected_line'" && {
        command-error "unexpected line '$1' found from the output"
    }
    return 0
}
