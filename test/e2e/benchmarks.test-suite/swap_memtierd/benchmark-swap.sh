#!/bin/bash

set -e -E

MEME_SIZE=${MEME_SIZE:-2G}
MEME_WRITE=${MEME_WRITE:-1}
SWAP_SIZE=${SWAP_SIZE:-4G}
COMP_ALGO=${COMP_ALGO:-lzo}

test-params() {
	echo "--- test parameters from environment variables and defaults ---"
	echo "test.env.MEME_SIZE:$MEME_SIZE"
	echo "test.env.MEME_WRITE:$MEME_WRITE"
	echo "test.env.SWAP_SIZE:$SWAP_SIZE"
	echo "test.env.COMP_ALGO:$COMP_ALGO"
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

echo ===== Clean up and reinitialize =====
test-params
pkill meme || true
pkill memtierd || true
echo N | tee /sys/module/zswap/parameters/enabled
grep -q zram /proc/swaps && swapoff /dev/zram0
grep -q zram /proc/modules && rmmod zram
modprobe zram || { echo "failed to load zram"; exit 1; }
echo "$COMP_ALGO" | tee /sys/module/zswap/parameters/compressor || { echo "bad COMP_ALGO=$COMP_ALGO"; exit 1; }
echo "$SWAP_SIZE" | tee /sys/block/zram0/disksize || { echo "bad SWAP_SIZE=$SWAP_SIZE"; exit 1; }
mkswap /dev/zram0
swapon /dev/zram0
echo Y | tee /sys/module/zswap/parameters/enabled
zram-params
zswap-params
swap-stats "start"
zram-stats "start"
zswap-stats "start"

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
process-stats "end" meme
process-stats "end" memtierd
perf-stats

echo ===== Clean up =====
kill $MEME_PID $MEMTIERD_PID
