# Test that guaranteed and burstable pods get the CPUs they require
# when there are enough CPUs available.

inject-affinities() {
    local var=$1 srcdst src dst hdr line
    shift
    if [ -z "$var" ] || [ -z "${!var}" ]; then
        return 0
    fi
    case "$var" in
        ANTI_*|*_ANTI_*) hdr="cri-resource-manager.intel.com/anti-affinity";;
        *)      hdr="cri-resource-manager.intel.com/affinity";;
    esac
    for srcdst in ${!var}; do
        src=${srcdst%:*}
        dst=${srcdst#*:}
        [ -n "$hdr" ] && { echo "    $hdr: |"; hdr=""; }
        line="$src: [ ${dst//,/, } ]"
        echo 1>&2 "* [affinity]: injecting affinity '$line'"
        echo "      $line"
    done
}

deref_keys() {
    eval "echo \${!$1[@]}"
}

deref_value() {
    eval "echo \${$1[$2]}"
}

inject-annotations() {
    local var=$1 values key value
    shift
    if [ -z "$var" ] || [ -z "${!var}" ]; then
        return 0
    fi
    for key in $(deref_keys ${!var}); do
        value=$(deref_value ${!var} $key)
        line="$key: $value"
        echo 1>&2 "* [annotation]: injecting annotation '$line'"
        echo "    $line"
    done
}

# pod0
# 4 containers, no affinities => spread out evenly over NUMA nodes
CONTCOUNT=4 CPU=1 create guaranteed+affinity
report allowed

verify \
    'nodes["pod0c0"] == {"node1"}' \
    'nodes["pod0c1"] == {"node2"}' \
    'nodes["pod0c2"] == {"node3"}' \
    'nodes["pod0c3"] == {"node0"}'

kubectl delete pods --all --now --wait

# pod1
# 4 containers, affinites [0,1], [2,3] => colocate c0,c1 in node1, c2,c3 in node2
CONTCOUNT=4 AFFINITIES="pod1c0:pod1c1 pod1c2:pod1c3" CPU=1 create guaranteed+affinity
report allowed

verify \
    'nodes["pod1c0"] == nodes["pod1c1"] == {"node1"}' \
    'nodes["pod1c2"] == nodes["pod1c3"] == {"node2"}'

kubectl delete pods --all --now --wait

# pod2
# 6 containers, anti-affinites 4:[0,1,2], 5:[0,2,3]
#   => don't co-locate 4 with {0,1,2}, or 5 with {0,2,3}
CONTCOUNT=6 ANTI_AFFINITIES="pod2c4:pod2c0,pod2c1,pod2c2 pod2c5:pod2c0,pod2c2,pod2c3" CPU=1 \
    create guaranteed+affinity
report allowed

verify \
    'disjoint_sets(nodes["pod2c4"], nodes["pod2c0"], nodes["pod2c1"], nodes["pod2c2"])' \
    'disjoint_sets(nodes["pod2c5"], nodes["pod2c0"], nodes["pod2c2"], nodes["pod2c3"])'
