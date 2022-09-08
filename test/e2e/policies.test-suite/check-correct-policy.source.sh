# This script does a policy check before the real test code is started.

cache_policy="$(vm-command-q "cat /var/lib/cri-resmgr/cache" | jq -r .PolicyName)"

cfg_policy=$(awk '/Active:/{print $2}' < "$cri_resmgr_cfg")

if [ -n "$cache_policy" ] && [ -n "$cfg_policy" ] && [ "$cache_policy" != "$cfg_policy" ]; then
    echo "cri-resmgr is been started with policy \"$cache_policy\", switching to \"$cfg_policy\""
    terminate cri-resmgr
    echo "destroying cri-resmgr cache with previous policy"
    vm-command "rm -rf /var/lib/cri-resmgr"
    launch cri-resmgr
fi
