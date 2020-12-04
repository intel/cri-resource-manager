# Redis parameters
REDIS_PASS=abc123xyz

# Background load parameters
STRESS_NG_CPUS=16 # workers per container
STRESS_NG_CONTS=8 # number of containers per pod
STRESS_NG_PODS=2 # number of pods

# BG_* are background loads
# CPU turbo licence level 2 (causes big drop on GHz) cannot be reached with stress-ng, but could be implemented with
# 1. ["avx-turbo", "--test=avx512_vlzcnt_t", "--min-threads=1", "--max-threads=1", "--iters=0"]
# 2. ["avx-turbo", "--test=avx512_vlzcnt_t", "--min-threads=1", "--max-threads=1", "--iters=0"]
# License level observed with:
# sudo perf stat --pid $(pidof avx-turbo) -e core_power.lvl0_turbo_license,core_power.lvl1_turbo_license,core_power.lvl2_turbo_license -- sleep 1
# In the following: "IPC" == Instructions Per Cycle
BG_NOLOAD=""
# BG_AVX_LL0="stress-ng --ipsec-mb $STRESS_NG_CPUS --ipsec-mb-feature avx512" # AVX, causing CPU turbo license level 0
# BG_AVX_LL1="stress-ng --ipsec-mb $STRESS_NG_CPUS --ipsec-mb-feature avx512" # AVX, causing CPU turbo license level 1
BG_SHM="stress-ng --shm $STRESS_NG_CPUS" # shared memory, memory bound (not causing 100% CPU load), IPC ~0.01-0.19
BG_MEMCPY="stress-ng --memcpy $STRESS_NG_CPUS" # memory bound; IPC =~ 0.15
BG_STREAM="stress-ng --stream $STRESS_NG_CPUS" # IPC =~ 0.49
BG_CPUJMP="stress-ng --cpu $STRESS_NG_CPUS --cpu-method jmp" # IPC ~3.6
BG_CPUALL="stress-ng --cpu $STRESS_NG_CPUS" # IPC ~1.8

# BM_* are benchmarks
bm_stress_ng_iters=10000
BM_MEMTIER="memtier-benchmark --server=redis-service --authenticate=$REDIS_PASS" # this is special case
# BM_MEMCPY="stress-ng --memcpy 1 --memcpy-ops $bm_stress_ng_iters" # IPC ~0.15
# BM_STREAM="stress-ng --stream 1 --stream-ops $bm_stress_ng_iters" # IPC ~0.49
# BM_JMP="stress-ng --cpu 1 --cpu-method jmp --cpu-ops $bm_stress_ng_iters" # IPC ~3.6
# BM_FFT="stress-ng --cpu 1 --cpu-method fft --cpu-ops $bm_stress_ng_iters" # IPC ~2.3
# BM_AVX_LL0="stress-ng --ipsec-mb 1 --ipsec-mb-feature avx2 --ipsec-mb-ops $bm_stress_ng_iters"
# BM_AVX_LL2="stress-ng --ipsec-mb 1 --ipsec-mb-feature avx512 --ipsec-mb-ops $bm_stress_ng_iters"

# Clean up
vm-command "kubectl delete jobs --all --now; kubectl delete deployment redis; kubectl delete service redis-service; kubectl delete secret redis; kubectl delete pods --all --now; true"

# Setup Redis
wait="" create redis-secret
CPU=4 MEM=32G CPULIM=8 MEMLIM=64G NAME=redis wait="Available" create redis
NAME=redis-service wait="" create redis-service

for bg_cmd in "${!BG_@}"; do
    # Reset counters in order to keep creating pod0...
    reset counters

    benchmark_output_dir="$OUTPUT_DIR/benchmark/$bg_cmd"
    mkdir -p "$benchmark_output_dir"

    # Start background noise
    if [[ "${!bg_cmd}" == "stress-ng "* ]]; then
        n="$STRESS_NG_PODS" ARGS="${!bg_cmd#stress-ng }" CONTCOUNT="$STRESS_NG_CONTS" CPU=50m MEM=50M CPULIM=$STRESS_NG_CPUS MEMLIM=1G wait_t=240s create stress-ng
        # Stabilize
        ( vm-run-until --timeout 60 "sh -c 'uptime; exit 1'" ) || echo "expected timeout"
    fi

    for bm_cmd in "${!BM_@}"; do
        for CPU in 4; do
            # Run benchmark
            if [[ "${!bm_cmd}" == "memtier-benchmark "* ]]; then
                AFFINITY=affinity CPU="$CPU" MEM="16G" CPULIM="$CPU" MEMLIM="24G" NAME=memtier-benchmark ARGS="${!bm_cmd#memtier-benchmark }" wait="Complete" wait_t="10m" create memtier-benchmark
                memtier_benchmark_pod="$(kubectl get pods | awk '/memtier-benchmark-/{print $1}')"
                kubectl logs "$memtier_benchmark_pod" | grep -A7 'ALL STATS' | tee "$benchmark_output_dir/$bm_cmd-affinity-cpu-$CPU.txt"
                kubectl delete jobs --all --now

                # AFFINITY=anti-affinity CPU="$CPU" MEM="16G" NAME=memtier-benchmark ARGS="${!bm_cmd#memtier-benchmark }" wait="Complete" wait_t="10m" create memtier-benchmark
                # memtier_benchmark_pod="$(kubectl get pods | awk '/memtier-benchmark-/{print $1}')"
                # kubectl logs "$memtier_benchmark_pod" | grep -A7 'ALL STATS' | tee "$benchmark_output_dir/$bm_cmd-antiaffinity-cpu-$CPU.txt"
                # kubectl delete jobs --all --now

            elif [[ "${!bm_cmd}" == "stress-ng "* ]]; then
                CPU="$CPU" MEM="200M" CPULIM="$STRESS_NG_CPUS" MEM="400M" NAME=stress-ng-benchmark ARGS="${!bm_cmd#stress-ng }" wait="Complete" wait_t="10m" create stress-ng-benchmark
                stress_ng_benchmark_pod="$(kubectl get pods | awk '/stress-ng-benchmark-/{print $1}')"
                kubectl logs "$stress_ng_benchmark_pod" | tee "$benchmark_output_dir/$bm_cmd-cpu-$CPU.txt"
                kubectl delete jobs --all --now
            fi
        done
    done

    # Stop background noise
    ( kubectl delete pods -l e2erole=bgload --now )
done
