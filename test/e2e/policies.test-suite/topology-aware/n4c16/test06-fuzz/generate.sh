#!/bin/bash

usage() {
    cat <<EOF
generate.sh - generate fuzz tests.

Configuring test generation with environment variables:
  TESTCOUNT=<NUM>       Number of generated test scripts than run in parallel.
  MEM=<NUM>             Memory [MB] available for test pods in the system.
  CPU=<NUM>             Non-reserved CPU [mCPU] available for test pods in the system.
  RESERVED_CPU=<NUM>    Reserved CPU [mCPU] available for test pods in the system.
  STEPS=<NUM>           Total number of test steps in all parallel tests.

  FMBT_IMAGE=<IMG:TAG>  Generate the test using fmbt from docker image IMG:TAG.
                        The default is fmbt-cli:latest.
EOF
    exit 0
}

if [ -n "$1" ]; then
    usage
fi

TESTCOUNT=${TESTCOUNT:-1}
MEM=${MEM:-7500}
# 950 mCPU taken by the control plane, split the remaining 15050 mCPU
# available for test pods to CPU and RESERVED_CPU pods.
CPU=${CPU:-14050}
RESERVED_CPU=${RESERVED_CPU:-1000}
STEPS=${STEPS:-100}
FMBT_IMAGE=${FMBT_IMAGE:-"fmbt-cli:latest"}

mem_per_test=$(( MEM / TESTCOUNT ))
cpu_per_test=$(( CPU / TESTCOUNT ))
reserved_cpu_per_test=$(( RESERVED_CPU / TESTCOUNT ))
steps_per_test=$(( STEPS / TESTCOUNT ))

# Check fmbt Docker image
docker run "$FMBT_IMAGE" fmbt --version 2>&1 | grep ^Version: || {
    echo "error: cannot run fmbt from Docker image '$FMBT_IMAGE'"
    echo "You can build the image locally by running:"
    echo "( cd /tmp && git clone --branch devel https://github.com/intel/fmbt && cd fmbt && docker build . -t $FMBT_IMAGE -f Dockerfile.fmbt-cli )"
    exit 1
}

cd "$(dirname "$0")" || {
    echo "cannot cd to the directory of $0"
    exit 1
}

for testnum in $(seq 1 "$TESTCOUNT"); do
    testid=$(( testnum - 1))
    sed -e "s/max_mem=.*/max_mem=${mem_per_test}/" \
        -e "s/max_cpu=.*/max_cpu=${cpu_per_test}/" \
        -e "s/max_reserved_cpu=.*/max_reserved_cpu=${reserved_cpu_per_test}/" \
        < fuzz.aal > tmp.fuzz.aal
    sed -e "s/fuzz\.aal/tmp.fuzz.aal/" \
        -e "s/pass = steps(.*/pass = steps(${steps_per_test})/" \
        < fuzz.fmbt.conf > tmp.fuzz.fmbt.conf
    OUTFILE=generated${testid}.sh
    echo "generating $OUTFILE..."
    docker run -v "$(pwd):/mnt/models" "$FMBT_IMAGE" sh -c 'cd /mnt/models; fmbt tmp.fuzz.fmbt.conf 2>/dev/null | fmbt-log -f STEP\$sn\$as\$al' | grep -v AAL | sed -e 's/^, /  /g' -e '/^STEP/! s/\(^.*\)/echo "TESTGEN: \1"/g' -e 's/^STEP\([0-9]*\)i:\(.*\)/echo "TESTGEN: STEP \1"; vm-command "date +%T.%N"; \2; vm-command "date +%T.%N"; kubectl get pods -A/g' | sed "s/\([^a-z0-9]\)\(r\?\)\(gu\|bu\|be\)\([0-9]\)/\1t${testid}\2\3\4/g" > "$OUTFILE"
done
