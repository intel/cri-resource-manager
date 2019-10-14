#!/bin/bash


RUNNER=${RUNNER:-"containerd"}
CRI_RESMGR_POLICY=${CRI_RESMGR_POLICY:-"null"}
CRI_RESMGR_POLICY_OPTIONS=${CRI_RESMGR_POLICY_OPTIONS:-"-dump='reset,full:.*' -dump-file=/tmp/cri.dump"}
CRI_RESMGR_DEBUG_OPTIONS=${CRI_RESMGR_DEBUG_OPTIONS:-""}



runtime_socket=$(find /run/ -iname $RUNNER.sock | head -1)
CRI_RESMGR_POLICY_OPTIONS+=" -runtime-socket=$runtime_socket -image-socket=$runtime_socket"


cp -f /bin/cri-resmgr /host/usr/local/bin/cri-resmgr

cp /config/cri-resource-manager.sysconf /host/etc/sysconfig/cri-resource-manager
cp /config/cri-resource-manager.service /host/etc/systemd/system/cri-resource-manager.service

sed -i "s|POLICY=.*|POLICY=$CRI_RESMGR_POLICY|" /host/etc/sysconfig/cri-resource-manager
sed -i "s|POLICY_OPTIONS=.*|POLICY_OPTIONS=$CRI_RESMGR_POLICY_OPTIONS|" /host/etc/sysconfig/cri-resource-manager
sed -i "s|DEBUG_OPTIONS=.*|DEBUG_OPTIONS=$CRI_RESMGR_DEBUG_OPTIONS|" /host/etc/sysconfig/cri-resource-manager

systemctl daemon-reload
systemctl restart cri-resource-manager

mkdir -p /host/etc/systemd/system/kubelet.service.d/
cat > /host/etc/systemd/system/kubelet.service.d/99-cri-resource-manager.conf <<EOF
[Service]
Environment=KUBELET_EXTRA_ARGS=
Environment=KUBELET_EXTRA_ARGS="--container-runtime remote --container-runtime-endpoint unix:///var/run/cri-resmgr/cri-resmgr.sock"
EOF

mv /var/lib/kubelet/kubeadm-flags.env /var/lib/kubelet/kubeadm-flags.env.bkp

systemctl daemon-reload
systemctl restart kubelet

#It is assumed this script will be called as a daemonset. As a result, do
# not return, otherwise the daemon will restart and rexecute the script
sleep infinity
