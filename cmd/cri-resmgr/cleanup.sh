#!/bin/bash

rm -f /etc/systemd/system/kubelet.service.d/99-cri-resource-manager.conf

if test -f "/var/lib/kubelet/kubeadm-flags.env.bkp"; then
    mv /var/lib/kubelet/kubeadm-flags.env.bkp /var/lib/kubelet/kubeadm-flags.env
fi

systemctl daemon-reload
systemctl restart kubelet
