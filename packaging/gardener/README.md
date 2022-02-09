# CRI-Resource-Manager extension for Gardener


**OUTDATED: we're in progress to use full fledged operator approach**

Note: This is work in progress and so far the only installed resources in shoot cluster are two configmaps for testing - in future they will be replaced with "installation" DaemonSet that will copy necessary binary and reconfigure and restart kubelet.

### Introduction

This charts will deploy CRI-resource-manager as proxy between kubectl and containerd for "shoot" clusters deployed by Gardener.


### Description

There are two charts:

- charts/**gardener-extension-cri-rm** - used to generate ControllerRegistration (CR) and ControllerDeployment (CD) objects in examples/ctrldeploy-ctrlreg.yaml by hacks/generate-controller-registation.sh
- charts/**cri-rm-installation** - internal chart that is included inside extension and used to install cri-resource-manager binary inside worker nodes in Shoot clusters - it is not meant to be run manually but rather deployed by *cri-rm-extension* chart

### Prerequisites

- working dir for `mkdir ~/work`
- *kubectl* 1.20+
- *helm* 3.7+
- *cri-resource-manager* is cloned to ~/work path (temporary ppalucki fork with "gardener" branch) like this:
    ```
    git clone https://github.com/ppalucki/cri-resource-manager/ ~/work/cri-resource-manager
    ```

## I. Getting started locally (with kind-based local gardener setup).

### Prepare local kind-based garden cluster

#### 1. Clone the gardener
```
mkdir -p ~/work/
git clone https://github.com/gardener/gardener ~/work/gardener
```

#### 2. Prepare kind cluster 

This is based on https://github.com/gardener/gardener/blob/master/docs/deployment/getting_started_locally.md


```
cd ~/work/gardener
make kind-up

kubectl cluster-info --context kind-gardener-local --kubeconfig /root/work/gardener/example/gardener-local/kind/kubeconfig
cp /root/work/gardener/example/gardener-local/kind/kubeconfig /root/work/gardener/example/provider-local/base/kubeconfig
# WARNING!: this overwrites your local kubeconfig
cp /root/work/gardener/example/gardener-local/kind/kubeconfig /root/.kube/config
```

Check everything is fine:
```
kubectl get nodes
```

####  3. Deploy local gardener

```
make gardener-up
```

Check that three gardener charts are installed:
```
helm list -n garden
```

### Deploy cri-rm extenion

#### 4. (Optional) Regenerate ctrldeploy-ctrlreg.yaml file:

```
cd ~/work/cri-resource-manager/packaging/gardener/
./hacks/generate-controller-registation.sh
```

#### 5. Deploy cri-rm-extension as Gardener extension using ControllerRegistration/ControllerDeployment

```
kubectl apply -f ~/work/cri-resource-manager/packaging/gardener/examples/ctrldeploy-ctrlreg.yaml
```

Checkout installed objects:
```
kubectl get controllerregistrations.core.gardener.cloud
kubectl get controllerdeployments.core.gardener.cloud
```
There should be 'cri-rm-extension   Extension/cri-rm-extension' resources visible alongside cri-rm-extension deployment.

but installation should not be yet available - there is not yet shoot cluster available.

```
kubectl get controllerinstallation.core.gardener.cloud
```

#### 5. Deploy shoot "local" cluster.

Build an image with extension and upload to local kind cluster
```
make docker-images

```


```
kubectl apply -f ~/work/cri-resource-manager/packaging/gardener/examples/shoot.yaml
```

Check that shoot cluster is ready:

```
kubectl get shoots.core.gardener.cloud -n garden-local --watch -o wide
```

#### 6. Verify that ManagedResources are properly installed in shoot 'garden' (seed class) and  'shoot--local--local' namespace



```
kubectl get managedresource -n garden | grep cri-rm-extension
kubectl get managedresource -n shoot--local--local | grep cri-rm-installation
```


#### 7. Check shoot cluster state

First get credentials to access shoot cluster:

``` 
kubectl -n garden-local get secret local.kubeconfig -o jsonpath={.data.kubeconfig} | base64 -d > /tmp/kubeconfig-shoot-local.yaml
# Check you can connect to node
kubectl --kubeconfig=/tmp/kubeconfig-shoot-local.yaml get nodes
```

Then check managed resources were created:
```
kubectl get managedresource -n garden | grep cri-rm-extension
kubectl get managedresource -n shoot--local--local cri-rm-installation -o yaml
```


