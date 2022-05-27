# Redis parameters
REDIS_PASS=abc123xyz

test_time_minutes=3
delay_between_rounds=30s
memtier_threads=1
test_time_seconds=$((test_time_minutes * 60))
test_timeout_minutes=$((test_time_minutes + 2))

# BM_* are benchmarks
BM_MEMTIER="memtier-benchmark --server=redis-service --authenticate=$REDIS_PASS --threads=${memtier_threads} --test-time=${test_time_seconds}"

# Clean up
vm-command "kubectl delete jobs --all --now; kubectl delete deployment redis; kubectl delete service redis-service; kubectl delete secret redis; kubectl delete pods --all --now; true"

vm-command "systemctl restart dbus"
vm-command "systemctl restart kubelet"

# Setup Redis
wait="" create redis-secret
CPU=4 MEM=8G CPULIM=8 MEMLIM=12G NAME=redis wait="Available" create redis
NAME=redis-service wait="" create redis-service

# Reset counters in order to keep creating pod0...
reset counters

benchmark_output_dir="$OUTPUT_DIR/benchmark/multi-memtier"
mkdir -p "$benchmark_output_dir"

export NAME=memtier-benchmark
for bm_cmd in "${!BM_@}"; do
    for CPU in 1 2; do
	for CONTCOUNT in 2 4 6 8 10 12 14; do
            # Run benchmark
            AFFINITY=affinity CPU="$CPU" MEM="4G" CPULIM="$CPU" MEMLIM="5G" ARGS="${!bm_cmd#memtier-benchmark }" wait="Complete" wait_t="${test_timeout_minutes}m" create memtier-benchmark-02
            memtier_benchmark_pod="$(kubectl get pods | awk '/memtier-benchmark-/{print $1}')"
	    outfile="$benchmark_output_dir/$bm_cmd-affinity-cpu-$CPU-contcount-$CONTCOUNT.txt"
	    rm -f $outfile
	    for contnum in $(seq 0 $((CONTCOUNT-1))); do
		contname=${NAME}c${contnum}
		echo "memtier benchmark CPU=$CPU log for $contname:"
		kubectl logs "$memtier_benchmark_pod" "$contname" | grep -A7 'ALL STATS' | tee -a $outfile
	    done
            kubectl delete jobs --all --now
	    # find average from all containers:
	    #============================================================================================================================
	    #Type         Ops/sec     Hits/sec   Misses/sec    Avg. Latency     p50 Latency     p99 Latency   p99.9 Latency       KB/sec
	    #----------------------------------------------------------------------------------------------------------------------------
	    sum_ops_sec=$(awk '/Totals/ {sum+=$2} END {print sum}' $outfile)
	    avg_ops_sec=$(awk '/Totals/ {sum+=$2;cnt++} END {printf("%d", sum/cnt)}' $outfile)
	    avg_latency=$(awk '/Totals/ {sum+=$5;cnt++} END {printf("%f", sum/cnt)}' $outfile)
	    avg_p50_lat=$(awk '/Totals/ {sum+=$6;cnt++} END {printf("%f", sum/cnt)}' $outfile)
	    avg_p99_lat=$(awk '/Totals/ {sum+=$7;cnt++} END {printf("%f", sum/cnt)}' $outfile)
	    printf "CPU=%2d CONTAINERS=%2d Ops/sec_sum:%7.0f Ops/sec_avg:%7.0f Latency:%f p50Latency:%f p99Latency:%f\n" \
		   $CPU $CONTCOUNT $sum_ops_sec $avg_ops_sec $avg_latency $avg_p50_lat $avg_p99_lat |tee -a $outfile
	    echo "sleep ${delay_between_rounds} between tests..."
	    echo "=================================================="
	    sleep ${delay_between_rounds}
	done
    done
done
echo "Use 'grep CONTAINERS output-of-this-session' to show one-line-per-round output"
