# Test that normal pods/containers scheduled on a CMK node
# are running in the shared pool, yet there are not as many
# CPUs as required.

cri_resmgr_cfg="$TEST_DIR/../cri-resmgr-static-pools.cfg" static-pools-relaunch-cri-resmgr

out ""
out "### Creating a guaranteed pod, 1 CPU, goes to the shared bool"
CPU=1 create cmk-tolerating-guaranteed
report allowed
verify 'cores["pod0c0"].issubset(shared_cores)'

out ""
out "### Creating next guaranteed pod, 2 CPUs, goes to the shared pool"
CPU=2 create cmk-tolerating-guaranteed
report allowed
verify 'cores["pod0c0"].issubset(shared_cores)' \
       'cores["pod1c0"].issubset(shared_cores)'

out ""
out "### Creating next guaranteed pod, 4 CPUs, goes to the shared pool"
CPU=4 create cmk-tolerating-guaranteed
report allowed
verify 'cores["pod0c0"].issubset(shared_cores)' \
       'cores["pod1c0"].issubset(shared_cores)' \
       'cores["pod2c0"].issubset(shared_cores)'

out ""
out "### Creating next guaranteed pod, 8 CPUs, goes to the shared pool"
CPU=6 create cmk-tolerating-guaranteed
report allowed
verify 'cores["pod0c0"].issubset(shared_cores)' \
       'cores["pod1c0"].issubset(shared_cores)' \
       'cores["pod2c0"].issubset(shared_cores)' \
       'cores["pod3c0"].issubset(shared_cores)'
