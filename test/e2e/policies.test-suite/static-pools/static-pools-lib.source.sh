# shellcheck disable=SC2148
static-pools-relaunch-cri-resmgr() {
    local webhook_running=0
    out "# Relaunching cri-resmgr and agent, launch webhook if not already running"

    vm-command-q "kubectl get mutatingwebhookconfiguration/cri-resmgr" >& /dev/null && {
        webhook_running=1
    }
    # cleanup
    terminate cri-resmgr
    terminate cri-resmgr-agent
    vm-command "rm -rf /var/lib/cri-resmgr"
    extended-resources remove cmk.intel.com/exclusive-cpus >/dev/null

    # launch again
    launch cri-resmgr-agent
    launch cri-resmgr
    vm-run-until "! kubectl get node | grep NotReady" ||
        error "kubectl node is NotReady after launching cri-resmgr-agent and cri-resmgr"
    if [ "$webhook_running" == 0 ]; then
        vm-command-q "[ -f webhook/webhook-deployment.yaml ]" ||
            install cri-resmgr-webhook
        launch cri-resmgr-webhook
    fi
}
