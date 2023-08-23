# Relaunch cri-resmgr so that it will listen to cri-resmgr-agent
cleanup() {
    vm-command "kubectl delete pod -n kube-system pod0 --now --wait --ignore-not-found; kubectl delete pods --all --now --wait; kubectl delete cm -n kube-system cri-resmgr-config.default"
    terminate cri-resmgr
    terminate cri-resmgr-agent
    vm-command "cri-resmgr -reset-policy; cri-resmgr -reset-config"
}

cleanup
cri_resmgr_config=fallback launch cri-resmgr
launch cri-resmgr-agent

# Create a pod to every pod pool in the default config:
# reserved, shared, singlecpu, dualcpu
# pod0: reserved
CPUREQ="" namespace=kube-system create podpools-busybox
# pod1: default
CPUREQ="" create podpools-busybox
# pod2: singlecpu
CPUREQ="1" POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: singlecpu" create podpools-busybox
# pod3, pod4, pod5, pod6: dualcpu (dualcpu 3 pods/pool, packed)
n=4 CPUREQ="1" POD_ANNOTATION="pool.podpools.cri-resource-manager.intel.com: dualcpu" create podpools-busybox
report allowed
verify "cpus['pod0c0'] == expected.cpus.reserved[0]" \
       "cpus['pod1c0'] == expected.cpus.default[0]" \
       "cpus['pod2c0'] == expected.cpus.singlecpu[0]" \
       "cpus['pod3c0'] == expected.cpus.dualcpu[0]" \
       "cpus['pod4c0'] == expected.cpus.dualcpu[0]" \
       "cpus['pod5c0'] == expected.cpus.dualcpu[0]" \
       "cpus['pod6c0'] == expected.cpus.dualcpu[1]"

echo "Switch to new configuration without singlecpu pools"
vm-put-file $(NAME=dualcpu CPU=2 MAXPODS=2 INSTANCES="100 %" instantiate podpools-configmap.yaml) podpools-dualcpu-configmap.yaml
kubectl apply -f podpools-dualcpu-configmap.yaml
sleep 5
report allowed
verify "cpus['pod0c0'] == expected.cpus.reserved[0]" `# reserved remains the same` \
       "len(cpus['pod1c0']) == 1" `# the default pool has only one CPU` \
       "cpus['pod2c0'] == cpus['pod1c0']" `# no singlecpu pool -> assign to default` \
       `# there are many dualcpu pools (1 out of 2 pods/pool, balanced)` \
       "len(cpus['pod3c0']) == 2" \
       "len(cpus['pod4c0']) == 2" \
       "len(cpus['pod5c0']) == 2" \
       "len(cpus['pod6c0']) == 2" \
       "disjoint_sets(cpus['pod3c0'], cpus['pod4c0'], cpus['pod5c0'], cpus['pod6c0'])"

echo "Negative test: try switching to an invalid configuration, check assignments have not changed"
vm-put-file $(NAME=borked CPU=130 MAXPODS=2 INSTANCES=1 instantiate podpools-configmap.yaml) podpools-borked-configmap.yaml
kubectl apply -f podpools-borked-configmap.yaml
sleep 5
report allowed
verify "cpus['pod0c0'] == {'cpu15'}" \
       "cpus['pod1c0'] == cpus['pod2c0']" \
       "disjoint_sets(cpus['pod3c0'], cpus['pod4c0'], cpus['pod5c0'], cpus['pod6c0'])" \

echo "After broken reconfiguration trial, switch to valid configuration without dualcpu pools"
# This configuration leaves no left-over CPUs for the default pool
# => the default pool will use the same CPUs as the reserved pool.
vm-put-file $(NAME=singlecpu CPU=1 MAXPODS=1 INSTANCES="100 %" instantiate podpools-configmap.yaml) podpools-dualcpu-configmap.yaml
kubectl apply -f podpools-dualcpu-configmap.yaml
sleep 5
report allowed

verify "cpus['pod0c0'] == expected.cpus.reserved[0]" `# reserved remains the same` \
       "cpus['pod1c0'] == expected.cpus.reserved[0]" `# the default pool equals to reserved` \
       "len(cpus['pod2c0']) == 1" `# pod2 in singlecpu[0]` \
       "disjoint_sets(cpus['pod2c0'], expected.cpus.reserved[0])" \
       `# all dualcpu pods endup into the default pool` \
       "cpus['pod3c0'] == cpus['pod4c0'] == cpus['pod5c0'] == cpus['pod6c0']" \
       "cpus['pod3c0'] == expected.cpus.reserved[0]"

echo "Not enough dualcpu pools for all running dualcpu pods, the rest fall back to the default pool"
vm-put-file $(NAME=dualcpu CPU=2 MAXPODS=1 INSTANCES="2" instantiate podpools-configmap.yaml) podpools-dualcpu-configmap.yaml
kubectl apply -f podpools-dualcpu-configmap.yaml
sleep 5
report allowed
pp cpus
verify "cpus['pod0c0'] == expected.cpus.reserved[0]" `# reserved remains the same` \
       "len(cpus['pod1c0']) == 9" `# the default pool` \
       "cpus['pod2c0'] == cpus['pod1c0']" `# no singlecpu pool -> assign to default` \
       `# two dualcpu pods go to dualcpu pools, two to the default pool` \
       "len([c for c in ['pod3c0', 'pod4c0', 'pod5c0', 'pod6c0'] if len(cpus[c])==2]) == 2" \
       "len([c for c in ['pod3c0', 'pod4c0', 'pod5c0', 'pod6c0'] if len(cpus[c])==9]) == 2"

# Clean up agent-delivered configuration setup as it might break tests
# that by default rely on forced configurations.
cleanup
launch cri-resmgr
launch cri-resmgr-agent
