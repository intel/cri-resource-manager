# CRI-Resource-Manager extension for Gardener

## Introduction



## Description

There are two charts:

- *cri-rm-extension* to install extension to garden cluster using ControllerRegistration and ControllerDeployment
- *cri-rm-installation* - internal chart that is included inside ControllerDeployment used to install cri-resource-manager binary inside worker nodes in Shoort clusters - it is not meant to be run manually but rather deployed by *cri-rm-extension* chart



### Generate extension chart


```
mkdir /tmp/work/
git clone https://github.com/gardener/gardener /tmp/work/gardener


# from charts directory
cd ~/work/cri-resource-manager/packaging/gardener/charts


/tmp/work/gardener/hack/generate-controller-registration.sh --optional cri-rm-extension cri-rm-installation v0.0.1 cri-rm-extension/templates/ctrldeploy-ctrlreg.yaml Extension:cri-rm-extension

# in case of tar unrecognize parameters (tar to old)
sed -i 's/--sort=name//' /tmp/work/gardener/hack/generate-controller-registration.sh
sed -i 's/--owner=root:0//' /tmp/work/gardener/hack/generate-controller-registration.sh
sed -i 's/--group=root:0//' /tmp/work/gardener/hack/generate-controller-registration.sh


```


## Getting started locally


Use kubectl plugins for context and config management:

Krew: https://krew.sigs.k8s.io/docs/user-guide/setup/install/

then this plugins:

```
kubectl krew install ctx
kubectl krew install config
kubectl krew install view-secret
kubectl krew install trace
```

Prepare local kind-based garden cluster


```
mkdir /tmp/work/

git clone https://github.com/gardener/gardener /tmp/work/gardener

cd /tmp/work/gardener

make kind-up
kubectl cluster-info --context kind-gardener-local --kubeconfig /root/work/gardener/example/gardener-local/kind/kubeconfig
cp /root/work/gardener/example/gardener-local/kind/kubeconfig /root/work/gardener/example/provider-local/base/kubeconfig
cp /root/work/gardener/example/gardener-local/kind/kubeconfig /root/.kube/config

kubectl get nodes
docker ps

kubectl ctx

```
