# This test is available only when using build flag BENCHMARK
terminate cri-resmgr
( cri_resmgr_cfg=${TEST_DIR}/balloons-instances.cfg cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr ) || {
    vm-command "grep 'this balloons policy build does not support benchmarking features' < cri-resmgr.output.txt" && {
        echo "WARNING:"
        echo "WARNING: Skipping $TEST_DIR."
        echo "WARNING: cri-resmgr is built without benchmarking build tag"
        echo "WARNING:"
        sleep 3
        exit 0
    }
    error "failed to start cri-resmgr"
}

# First two fixed instances are created at init.
verify-metrics-has-line 'balloon="fixed\[0\].*mems="0"'
verify-metrics-has-line 'balloon="fixed\[1\].*mems="2,3"'
verify-metrics-has-line 'balloon="fixed\[1\].*p1d0n3'
CPU=250m MEM=100M n=4 create guaranteed

# Last two fixed instances are created on-demand when creating containers.
verify-metrics-has-line 'balloon="fixed\[2\].*numas="p1d0n2"'
verify-metrics-has-line 'balloon="fixed\[2\].*mems="1"'
verify-metrics-has-line 'balloon="fixed\[3\].*packages="p0"'
verify 'packages["pod0c0"] == {"package0"}' \
       'mems["pod0c0"] == {"node0"}' \
       'cpus["pod1c0"] == {"cpu14", "cpu15"}' \
       'mems["pod1c0"] == {"node2", "node3"}' \
       'packages["pod2c0"] == {"package1"}' \
       'mems["pod2c0"] == {"node1"}' \
       'packages["pod3c0"] == {"package0"}'

terminate cri-resmgr
launch cri-resmgr
