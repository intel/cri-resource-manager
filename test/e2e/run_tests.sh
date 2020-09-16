#!/bin/bash

TESTS_DIR="$1"
RUN_SH="${0%/*}/run.sh"

usage() {
    echo "Usage: run_tests.sh TESTS_DIR"
    echo "TESTS_DIR is expected to be structured as POLICY/TOPOLOGY/TEST with files:"
    echo "POLICY/cri-resmgr.cfg: configuration of cri-resmgr"
    echo "POLICY/TOPOLOGY/topology.var.json: contents of the topology variable for run.sh"
    echo "POLICY/TOPOLOGY/TEST/code.var.sh: contents of the code var (that is, test script)"
}

error() {
    (echo ""; echo "error: $1" ) >&2
    exit 1
}

export_var_files() {
    local var_file_dir="$1"
    local var_filepath
    local var_file_name
    local var_name
    for var_filepath in "$var_file_dir"/*.var "$var_file_dir"/*.var.*; do
        if ! [ -f "$var_filepath" ] || [[ "$var_filepath" == *"~" ]]; then
            continue
        fi
        var_file_name=$(basename "$var_filepath")
        var_name=${var_file_name%%.var*}
        echo "exporting $var_name from $var_filepath"
        export "$var_name"="$(< "$var_filepath")"
    done
}

if [ -z "$TESTS_DIR" ] || [ "$TESTS_DIR" == "help" ] || [ "$TESTS_DIR" == "--help" ]; then
    usage
    error "missing TESTS_DIR"
fi

if ! [ -d "$TESTS_DIR" ]; then
    error "bad TESTS_DIR: \"$TESTS_DIR\""
fi

# Find TESTS_DIR root by looking for POLICY_DIR/*.cfg. If TESTS_DIR was not the
# root dir, then execute tests only under TESTS_DIR.
if compgen -G "$TESTS_DIR/*/*.cfg" >/dev/null; then
    TESTS_ROOT_DIR="$TESTS_DIR"
elif compgen -G "$TESTS_DIR/../*/*.cfg" >/dev/null; then
    TESTS_ROOT_DIR=$(realpath "$TESTS_DIR/..")
    TESTS_POLICY_FILTER=$(basename "${TESTS_DIR}")
elif compgen -G "$TESTS_DIR/../../*/*.cfg" >/dev/null; then
    TESTS_ROOT_DIR=$(realpath "$TESTS_DIR/../..")
    TESTS_POLICY_FILTER=$(basename "$(dirname "${TESTS_DIR}")")
    TESTS_TOPOLOGY_FILTER=$(basename "${TESTS_DIR}")
elif compgen -G "$TESTS_DIR/../../../*/*.cfg" >/dev/null; then
    TESTS_ROOT_DIR=$(realpath "$TESTS_DIR/../../..")
    TESTS_POLICY_FILTER=$(basename "$(dirname "$(dirname "${TESTS_DIR}")")")
    TESTS_TOPOLOGY_FILTER=$(basename "$(dirname "${TESTS_DIR}")")
    TESTS_TEST_FILTER=$(basename "${TESTS_DIR}")
else
    error "TESTS_DIR=\"$TESTS_DIR\" is invalid tests/policy/topology/test dir: *.cfg not found"
fi

echo "Running tests matching:"
echo "    TESTS_ROOT_DIR=$TESTS_ROOT_DIR"
echo "    TESTS_POLICY_FILTER=$TESTS_POLICY_FILTER"
echo "    TESTS_TOPOLOGY_FILTER=$TESTS_TOPOLOGY_FILTER"
echo "    TESTS_TEST_FILTER=$TESTS_TEST_FILTER"

cleanup() {
    rm -rf "$summary_dir"
}
summary_dir=$(mktemp -d)
trap cleanup TERM EXIT QUIT

summary_file="$summary_dir/summary.txt"
echo -n "" > "$summary_file"

for POLICY_DIR in "$TESTS_ROOT_DIR"/*; do
    if ! [ -d "$POLICY_DIR" ]; then
        continue
    fi
    if ! [[ "$(basename "$POLICY_DIR")" =~ .*"$TESTS_POLICY_FILTER".* ]]; then
        continue
    fi
    # Run exports in subshells so that variables exported for previous
    # tests do not affect any other tests.
    (
        for CFG_FILE in "$POLICY_DIR"/*.cfg; do
            if ! [ -f "$CFG_FILE" ]; then
                error "cannot find cri-resmgr configuration $POLICY_DIR/*.cfg"
            fi
            export cri_resmgr_cfg=$CFG_FILE
        done
        export_var_files "$POLICY_DIR"
        for TOPOLOGY_DIR in "$POLICY_DIR"/*; do
            if ! [ -d "$TOPOLOGY_DIR" ]; then
                continue
            fi
            if ! [[ "$(basename "$TOPOLOGY_DIR")" =~ .*"$TESTS_TOPOLOGY_FILTER".* ]]; then
                continue
            fi
            (
                export_var_files "$TOPOLOGY_DIR"
                vm="$(basename "$TOPOLOGY_DIR")"
                export vm
                for TEST_DIR in "$TOPOLOGY_DIR"/*; do
                    if ! [ -d "$TEST_DIR" ]; then
                        continue
                    fi
                    if ! [[ "$(basename "$TEST_DIR")" =~ .*"$TESTS_TEST_FILTER".* ]]; then
                        continue
                    fi
                    (
                        export_var_files "$TEST_DIR"
                        export outdir="$TEST_DIR/output"
                        mkdir -p "$outdir"
                        echo "Run $(basename "$TEST_DIR")"
                        "$RUN_SH" test 2>&1 | tee "$outdir/run.sh.output"
                        test_name="$(basename "$POLICY_DIR")/$(basename "$TOPOLOGY_DIR")/$(basename "$TEST_DIR")"
                        if grep -q "Test verdict: PASS" "$outdir/run.sh.output"; then
                            echo "PASS $test_name" >> "$summary_file"
                        elif grep -q "Test verdict: FAIL" "$outdir/run.sh.output"; then
                            echo "FAIL $test_name" >> "$summary_file"
                        else
                            echo "ERROR $test_name" >> "$summary_file"
                        fi
                    )
                done
            )
        done
    )
done

echo ""
echo "Tests summary:"
cat "$summary_file"
if grep -q ERROR "$summary_file" || grep -q FAIL "$summary_file"; then
    exit 1
fi
