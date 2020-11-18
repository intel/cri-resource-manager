# Test migrating memory pages from DRAM to PMEM.
# - Memory pages that are written once and never read
#   must be migrated to PMEM and must stay there.
# - Memory pages that are actively written and read
#   must not be migrated to PMEM.
# - Migration speed is as configured.

# Relaunch cri-resmgr with dynamic page demotion configuration.
cri_resmgr_cfg=$TEST_DIR/cri-resmgr-dynamic-page-demotion.cfg
terminate cri-resmgr
launch cri-resmgr

# Different memory usage profiles are implemented with awk
# in order to manage with the same busybox image as other tests.
# Memory size parameters for the busybox memory load pod:
# - BSIZE: Block size in bytes (length of each stored string)
#   The larger the block the faster the awk goes through its memory.
#   If too large, memory for strings is no more allocated from heap
#   which makes page tracking harder and breaks this test.
# - WORN: Write Once Read Never
# - WORM: Write Once Read Many
# - WMRN: Write Many Read Never
# - WMRM: Write Many Read Many
PRINT_WRBYTES_IF="wr%1000==0 && wr<10000"
CPU=500m
BSIZE=4096
awkmem=2M
WORN=$awkmem WORM=$awkmem WMRN=$awkmem WMRM=$awkmem create bb-memload

# Calculate page migration speed from cri-resmgr configuration.
pages_per_second_per_process="$(awk '
    /MaxPageMoveCount:/{mpmc=$2}
    /PageMoveInterval:/{gsub(/[^0-9]/, "", $2); pmi=$2}
    END{print mpmc/pmi}
    ' < "$cri_resmgr_cfg")"

# After how many rounds (seconds) first migrations should be visible.
first_migrations_visible="$(awk '
    /PageScanInterval:/{gsub(/[^0-9]/, "", $2); print $2+1}
    ' < "$cri_resmgr_cfg")"

# Expected migrated number of pages when fully migrated.
pages_error_margin=100
fully_migrated_threshold=$(( ${awkmem%M} * 1024 * 1024 / 4096 - pages_error_margin ))

# Maximum number of pages in PMEM when not migrated.
not_migrated_threshold=$pages_error_margin

# Watch memory page locations and validate results.
memload_stats="$OUTPUT_DIR/memload-stats.txt"
echo -n "" > "$memload_stats"
max_rounds=30
round=0
declare -A pmem_pages_prev # number of pages in PMEM in previous round
for wxrx in wmrm wmrn worm worn; do
    pmem_pages_prev[$wxrx]=0
done
while (( round < max_rounds )); do
    vm-command-q '
       cat /sys/devices/system/node/node[0-7]/meminfo | awk "/Active:/{a[\$2]=(\$4/1024)}END{s=\"active mem\";for(n=0;n<8;n++){s=sprintf(\"%s N%d=%.0fM\",s,n,a[n])}print s}"
       for p in $(pidof awk); do
           awkinfo=$(grep -a -o -E w[om]r[nm] /proc/$p/cmdline | head -n 1)
           rss=$(awk "/VmRSS:/{print \$2}" < /proc/$p/status);
           pages=$(echo $(grep -v file= /proc/$p/numa_maps | tr " " "\n" | awk -F= "/N([0-9])/{s[\$1]+=\$2}END{for(n=0;n<8;n++)if (s[\"N\"n]>0)print \"N\"n\"=\"s[\"N\"n]}"))
           echo "$awkinfo" pid "$p" VmRSS "$rss" kB, "pages:" "$pages"
       done' | while read line; do echo "round $round $line"; done | tee -a "$memload_stats"

    echo "validating..."

    # Check that at least something has migrated after scan period.
    if (( round > first_migrations_visible )); then
        grep -q -E 'pages:.*N[4-7]' "$memload_stats" ||
            error "any of the awk processes was not migrated to PMEM in time"
    fi

    # Validate PMEM page migration speed.
    # Allow double the configured speed because stats polling interval > 1s.
    for wxrx in wmrm wmrn worm worn; do
        pmem_pages_now="$(awk -F'[ =]' "BEGIN{pmem=0}/round $round $wxrx .*pages: N[0-3].* N[4-7]/{pmem+=\$13}END{print pmem}" < "$memload_stats")"
        if (( pmem_pages_now - pmem_pages_prev[$wxrx] > 2 * pages_per_second_per_process )); then
            error "number of PMEM pages of $wxrx grew too quickly on this round"
        fi
        pmem_pages_prev[$wxrx]=$pmem_pages_now
    done

    # Check that write-once-read-never (worn) has migrated and stays in PMEM.
    if (( round > 20 )); then
        worn_pmem_pages="$(awk -F'[ =]' "/round $round worn .*pages: N[0-3].* N[4-7]/{pmem+=\$13}END{print pmem}" < "$memload_stats")"
        if (( worn_pmem_pages < fully_migrated_threshold )); then
            error "write-once-read-never was expected to end up and stay in PMEM, but only $worn_pmem_pages pages in PMEM."
        fi
    fi

    # Check that write-many-read-many and -read-never (wmrm and wmrn) stay in DRAM.
    for wmrx in wmrm wmrn; do
        wmrx_pmem_pages="$(awk -F'[ =]' "/round $round $wmrx .*pages: N[0-3].* N[4-7]/{pmem+=\$13}END{print pmem}" < "$memload_stats")"
        if (( wmrx_pmem_pages > not_migrated_threshold )); then
            error "$wmrx was expected to stay in DRAM, but $wmrx_pmem_pages pages migrated to PMEM."
        fi
    done

    sleep 1 >/dev/null
    round=$(( round + 1 ))
done
echo "All rounds were good."
kubectl delete pods --all --now
