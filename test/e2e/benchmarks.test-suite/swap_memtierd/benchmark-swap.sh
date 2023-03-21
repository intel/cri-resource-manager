#!/bin/bash

set -e -E

MEME_SIZE=${MEME_SIZE:-2G}
MEME_WRITE=${MEME_WRITE:-1}
SWAP_SIZE=${SWAP_SIZE:-4G}
COMP_ALGO=${COMP_ALGO:-lzo}
USE_ZSWAP=${USE_ZSWAP:-0}

if [ "$USE_ZSWAP" != "0" ] && [ "$USE_ZSWAP" != "1" ]; then
	echo "invalid USE_ZSWAP=$USE_ZSWAP, expected 0 or 1" >&2
	exit 1
fi

test-params() {
	echo "--- test parameters from environment variables and defaults ---"
	echo "test.env.MEME_SIZE:$MEME_SIZE"
	echo "test.env.MEME_WRITE:$MEME_WRITE"
	echo "test.env.SWAP_SIZE:$SWAP_SIZE"
	echo "test.env.COMP_ALGO:$COMP_ALGO"
}

sys-params() {
	echo "--- system parameters ---"
	echo "sys.hugepages:$(< /sys/kernel/mm/transparent_hugepage/enabled)"
	echo "sys.overcommit_memory:$(< /proc/sys/vm/overcommit_memory)"
	if [ -f /sys/module/iaa_crypto/parameters/iaa_crypto_enable ]; then
		echo "sys.iaa_crypto_enable:$(< /sys/module/iaa_crypto/parameters/iaa_crypto_enable)"
	else
		echo "sys.iaa_crypto_enable:ERROR"
	fi
}

zram-params() {
	local dir=/sys/block/zram0
	echo "--- zram parameters from $dir ---"
	grep -s . $dir/* | sed "s:$dir/:zram.params.:g"
}

zram-stats() {
	local file=/sys/block/zram0/mm_stat
	echo "--- zram statistics from $file ---"
	awk -v "t=$1" '{
		print t".zram.stats.orig_data_size:"$1;
		print t".zram.stats.compr_data_size:"$2;
		print t".zram.stats.mem_used_total:"$3;
		print t".zram.stats.mem_limit:"$4;
		print t".zram.stats.mem_used_max:"$5;
		print t".zram.stats.same_pages:"$6;
		print t".zram.stats.pages_compacted:"$7;
	}' < $file
}

zswap-params() {
	local dir=/sys/module/zswap/parameters
	echo "--- zswap parameters ---"
	grep -R . $dir | sed "s:$dir/:zswap.params.:g"
}

zswap-stats() {
	local dir=/sys/kernel/debug/zswap
	echo "--- zswap statistics $1 ---"
	grep -R . $dir | sed "s:$dir/:$1.zswap.stats.:g"
}

swap-stats() {
	local file=/proc/swaps
	echo "--- swap statistics $1 from $file ---"
	awk -v "t=$1" '/zram0/{
		print t".swap.stats.size:"$3
		print t".swap.stats.used:"$4
	}' < $file
}

process-stats() {
	local process=$2
	local pid
        pid=$(pidof "$process")
	echo "--- $process (pid: $pid) statistics $1 ---"
	awk -v "p=$process" -v "t=$1" '{
		utime=$14*10;
		stime=$15*10;
		print t"."p".stats.utime.ms:"utime;
		print t"."p".stats.stime.ms:"stime;
	}' < /proc/"$pid"/stat
}

perf-stats() {
	echo "--- perf statistics from /tmp/perf.* ---"
	awk '/sys_exit_process_madvise/{print "process_madvice.duration.ms:"$1}' < /tmp/perf.process_madvice.txt
	awk '/instructions:u/{print "global.instructions.count:"$1}' < /tmp/perf.instructions.txt | sed 's/,//g'
	awk '/elapsed/{print "global.instructions.interval.s:"$1}' < /tmp/perf.instructions.txt
}

iaa-stats() {
	local dir=/sys/kernel/debug/iaa-crypto
	echo "--- iaa statistics ---"
	if ! [ -d "$dir" ]; then
		echo "$1.iaa.stats:ERROR-missing-$dir"
		return 0
	fi
	grep . $dir/total* | sed "s:$dir/:$1.iaa.stats.:g"
	if [ -f "$dir/wq_stats" ]; then
		awk -v "t=$1" '/id:/{id=$2;name=""}/name:/{name=$2}/_(bytes|calls):/{print t".iaa.dev"id".wq"name"."$1""$2}' < "$dir/wq_stats"
	else
		echo "$1.iaa.devX.wqY:ERROR-missing-$dir/wq_stats"
	fi
}

echo ===== Clean up =====
test-params
pkill meme || true
pkill memtierd || true
echo N | tee /sys/module/zswap/parameters/enabled
grep -q zram /proc/swaps && swapoff /dev/zram0
grep -q zram /proc/modules && rmmod zram
if [ -f /sys/module/iaa_crypto/parameters/iaa_crypto_enable ]; then
	echo 0 > /sys/module/iaa_crypto/parameters/iaa_crypto_enable
fi

echo ===== Initialize =====
modprobe zram || { echo "failed to load zram"; exit 1; }
if [ "$USE_ZSWAP" == "0" ]; then
    echo "$COMP_ALGO" | tee /sys/block/zram0/comp_algorithm
else
    echo "$COMP_ALGO" | tee /sys/module/zswap/parameters/compressor || { echo "bad COMP_ALGO=$COMP_ALGO"; exit 1; }
    echo 50 > /sys/module/zswap/parameters/max_pool_percent
    echo zsmalloc > /sys/module/zswap/parameters/zpool
    echo 0 > /sys/module/zswap/parameters/same_filled_pages_enabled
fi
echo never > /sys/kernel/mm/transparent_hugepage/enabled
echo 1 > /proc/sys/vm/overcommit_memory

if [[ "$COMP_ALGO" == *"iaa"* ]]; then
	if [ -f /sys/module/iaa_crypto/parameters/iaa_crypto_enable ]; then
		echo 1 > /sys/module/iaa_crypto/parameters/iaa_crypto_enable
		echo 0 > /sys/kernel/debug/iaa-crypto/stats_reset
	else
		echo ERROR: cannot enable iaa_crypto
	fi
fi

echo "$SWAP_SIZE" | tee /sys/block/zram0/disksize || { echo "bad SWAP_SIZE=$SWAP_SIZE"; exit 1; }
mkswap /dev/zram0
swapon /dev/zram0
if [ "$USE_ZSWAP" == "1" ]; then
    echo Y | tee /sys/module/zswap/parameters/enabled
fi
sys-params
zram-params
zswap-params
swap-stats "start"
zram-stats "start"
zswap-stats "start"
iaa-stats "start"

perf trace -e syscalls:sys_*_process_madvise --max-events 2 >&/tmp/perf.process_madvice.txt &

echo ===== Allocate memory and swap it out =====
echo "Launching meme that allocates $MEME_SIZE of memory..."
meme -bs "$MEME_SIZE" -bws "$MEME_SIZE" -bwc "$MEME_WRITE" -bwi 10m -brc 0 -ttl 10m < /dev/null & MEME_PID=$!
sleep 5
echo "Launching memtierd to swap out meme... (you will see memtierd prompts...)"
perf stat -e instructions:u -a sleep 5 >&/tmp/perf.instructions.txt &
( echo "swap -pid $MEME_PID -out"; sleep 10; echo "swap -pid $MEME_PID -status"; sleep 10m ) | memtierd -prompt & MEMTIERD_PID=$!
sleep 15
echo ""
echo "done."

echo ===== Collect data =====
swap-stats "end"
zram-stats "end"
zswap-stats "end"
iaa-stats "end"
process-stats "end" meme
process-stats "end" memtierd
perf-stats

echo ===== Clean up =====
kill $MEME_PID $MEMTIERD_PID
