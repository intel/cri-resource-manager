zram-swap() {
    # Usage: memtierd-zram-swap <SIZE>
    #        memtierd-zram-swap off
    if [[ -z "$1" ]]; then
        error "memtierd-zswap: missing SIZE or 'off'"
    fi
    if [[ "$1" != "off" ]]; then
        vm-command "lsmod | grep ^zram || modprobe zram" || return 1
        vm-command "echo '$1' > /sys/block/zram0/disksize" || return 1
        vm-command "mkswap /dev/zram0" || return 1
        vm-command "swapon /dev/zram0" || return 1
        vm-command "cat /proc/swaps"
    else
        vm-command "swapoff /dev/zram0" || return 1
        vm-command "rmmod zram" || return 1
    fi
}

zram-install() {
    vm-command "
    [ -d /sys/block/zram0 ] || ( find /lib/modules -name zram.ko | grep zram.ko ) || {
        grep -i ubuntu /etc/os-release && DEBIAN_FRONTEND=noninteractive apt install -y linux-modules-extra-\$(uname -r)
    }"
}
