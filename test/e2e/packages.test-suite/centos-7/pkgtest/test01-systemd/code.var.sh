# Reconfigure journald to allow very verbose logging.
vm-command "sed -i 's/^#RateLimit/RateLimit/g;s/Burst=.*$/Burst=10000/g' /etc/systemd/journald.conf"
vm-command "systemctl restart systemd-journald"

# Clear cri-resmgr output from previous runs.
vm-command "journalctl --vacuum-time=1s"

# Create a pod.
create besteffort

# Verify that new pod was created by systemd-managed cri-resource-manager.
vm-command "journalctl -xeu cri-resource-manager | grep 'StartContainer: starting container pod0:pod0c0'" || {
    command-error "failed to verify that systemd-managed cri-resource-manager launched the pod"
}
