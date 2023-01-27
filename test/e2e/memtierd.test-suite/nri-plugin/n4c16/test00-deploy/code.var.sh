MEME_IMAGE=$(docker images | grep meme | head -n 1 | awk '{print $1":"$2}')
if [ -z "$MEME_IMAGE" ]; then
    error "meme docker image missing on host. Run 'make image-meme' first."
fi
vm-command "ctr -n k8s.io images ls | grep $MEME_IMAGE" || \
    vm-put-docker-image "$MEME_IMAGE" || \
    error "image $MEME_IMAGE not found on vm and uploading it form host failed"

# TODO: deploy memtierd NRI-plugin

# Create meme workload that continuously writes 100M out of 1G mem.
ANN0="policy.memtierd.intel.com: \"age\""
ANN1="swapoutms.age.memtierd.intel.com: \"10000\""
MEME_BS=1G MEME_BWC=1 MEME_BWS=100M
create meme

sleep 1
# TODO: check that VmSwap > 900M once swapoutms has passed.
vm-command "grep Vm /proc/\$(pidof meme)/status"

vm-command "kubectl delete pods --all --now"
