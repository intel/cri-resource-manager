damon-idlepage-setup() {
    if [ "$distro" != "debian-sid" ]; then
        error "damon-idlepage-setup is implemented only for distro=debian-sid"
    fi
    # Clone Linux kernel and setup kernel development environment
    vm-command "[ -d linux ]" || vm-install-kernel-dev || {
            error "failed to install kernel development environment"
        }

    # Patch kernel configuration: enable DAMON and idle page tracking
    if ! vm-command "[ -f linux/.config.without-patches ]"; then
        vm-command "cp linux/.config linux/.config.without-patches"

        cat <<EOF |
--- .config 2023-02-08 09:35:25.298783387 +0000
+++ .config 2023-02-08 09:38:43.546783387 +0000
@@ -1127,7 +1147,13 @@
 #
 # Data Access Monitoring
 #
-# CONFIG_DAMON is not set
+CONFIG_DAMON=y
+CONFIG_DAMON_VADDR=y
+CONFIG_DAMON_PADDR=y
+CONFIG_DAMON_SYSFS=y
+CONFIG_DAMON_DBGFS=y
+CONFIG_DAMON_RECLAIM=y
+CONFIG_DAMON_LRU_SORT=y
 # end of Data Access Monitoring
 # end of Memory Management options

EOF
        vm-pipe-to-file "linux/config.enable-damon.patch"

        cat <<EOF |
--- .config 2023-02-08 09:35:25.298783387 +0000
+++ .config 2023-02-08 09:38:43.546783387 +0000
@@ -1094,7 +1114,8 @@
 CONFIG_MEM_SOFT_DIRTY=y
 CONFIG_GENERIC_EARLY_IOREMAP=y
 CONFIG_DEFERRED_STRUCT_PAGE_INIT=y
-# CONFIG_IDLE_PAGE_TRACKING is not set
+CONFIG_PAGE_IDLE_FLAG=y
+CONFIG_IDLE_PAGE_TRACKING=y
 CONFIG_ARCH_HAS_CACHE_LINE_SIZE=y
 CONFIG_ARCH_HAS_CURRENT_STACK_POINTER=y
 CONFIG_ARCH_HAS_PTE_DEVMAP=y
EOF
        vm-pipe-to-file "linux/config.enable-idlepage.patch"

        vm-command "cd linux; patch < config.enable-damon.patch; patch < config.enable-idlepage.patch" || {
            command-error "patching kernel configuration failed"
        }

        vm-command "cd linux; nice make -j8 bindeb-pkg" || {
            error "building debian packages failed"
        }
        vm-command 'echo $(ls linux-image-*.deb | grep -v dbg)'
        vm-reboot
    fi
}
