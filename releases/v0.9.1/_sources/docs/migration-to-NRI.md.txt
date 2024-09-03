# Migrating from CRI-RM to NRI

## Prerequisities

- Up and running CRI Resource Manager
- One of the two supported policies in use: balloons or topology-aware.
- For other policies a little bit more work is required and the policies need to be 'ported'. This can be done by just following the example of how balloons or topology-aware were converted.

## Steps for an initial/basic migration test

### Containerd

Replace the containerd version in the system with 1.7 or newer version (NRI server not supported in older versions).

Replace kubelet's --container-runtime-endpoint=/var/run/cri-resmgr/cri-resmgr.sock with --container-runtime-endpoint=/var/run/containerd/containerd.sock

Replacing the runtime endpoint on a node that was setup using Kubeadm:
```console
# Get the Kubelet args
systemctl cat kubelet <- Look for: EnvironmentFile=/.../kubeadm-flags.env

vim /.../kubeadm-flags.env
  KUBELET_KUBEADM_ARGS="--container-runtime-endpoint=unix:///var/run/containerd/containerd.sock --pod-infra-container-image=registry.k8s.io/pause:3.9"

vim /etc/sysconfig/kubelet
  KUBELET_EXTRA_ARGS= --container-runtime-endpoint=/var/run/containerd/containerd.sock <- Remember this aswell

systemctl restart kubelet
```

Edit the containerd config file and look for the section [plugins."io.containerd.nri.v1.nri"] and replace "disable = true" with "disable = false":
```console
vim /etc/containerd/config.toml
```
```toml
[plugins."io.containerd.nri.v1.nri"]
  disable = false
  disable_connections = false
  plugin_config_path = "/etc/nri/conf.d"
  plugin_path = "/opt/nri/plugins"
  plugin_registration_timeout = "5s"
  plugin_request_timeout = "2s"
  socket_path = "/var/run/nri/nri.sock"
```
```console
systemctl restart containerd
```

### CRI-O

Ensure that crio version 1.26.2 or newer is used.

Replace kubelet's --container-runtime-endpoint=/var/run/cri-resmgr/cri-resmgr.sock with --container-runtime-endpoint=/var/run/crio/crio.sock

Replacing the runtime endpoint on a node that was setup using Kubeadm:
```console
# Get the Kubelet args
systemctl cat kubelet <- Look for: EnvironmentFile=/.../kubeadm-flags.env

vim /.../kubeadm-flags.env
  KUBELET_KUBEADM_ARGS="--container-runtime-endpoint=unix:///var/run/crio/crio.sock --pod-infra-container-image=registry.k8s.io/pause:3.9"

vim /etc/sysconfig/kubelet
  KUBELET_EXTRA_ARGS= --container-runtime-endpoint=/var/run/crio/crio.sock <- Remember this aswell

systemctl restart kubelet
```

Enable NRI:
```console
CRIO_CONF=/etc/crio/crio.conf
cp $CRIO_CONF $CRIO_CONF.orig
crio --enable-nri config > $CRIO_CONF
systemctl restart crio
```

### Build the NRI policies

```console
git clone https://github.com/containers/nri-plugins.git
cd nri-plugins
make
# Build the images, specify your image repo to easily push the image later.
make images IMAGE_REPO=my-repo IMAGE_VERSION=my-tag
```

### Create required CRDs

```console
kubectl apply -f deployment/base/crds/noderesourcetopology_crd.yaml
```

### Import the image of the NRI plugin you want to run

<b>Containerd</b>

```console
ctr -n k8s.io images import build/images/nri-resmgr-topology-aware-image-*.tar
```

<b>CRI-O</b>

See the section [below](#steps-for-a-more-real-life-migration-using-self-hosted-image-repository) for instructions on how to push the images to a registry, then pull from there.

### Deploy the plugin

  ```console
  kubectl apply -f build/images/nri-resmgr-topology-aware-deployment.yaml
  ```

### Deploy a test pod

```console
kubectl run mypod --image busybox -- sleep inf
kubectl exec mypod  -- grep allowed_list: /proc/self/status
```

### See the resources the pod got assigned with

```console
kubectl exec $pod -c $container  -- grep allowed_list: /proc/self/status
# Output should look similar to the output of CRI-RM
```

## Steps for a more real-life migration using self-hosted image repository

- Same steps as above for enabling NRI with Containerd/CRI-O and building the images.
- Push the images built to your repository:
  ```console
  # Replace my-repo and my-tag with the IMAGE_REPO and IMAGE_VERSION you specified when building the images with make images
  docker push my-repo:my-tag
  ```

- Remember to change the image name & pull policy in the plugins .yaml file to match your registyr and image, ex:
  ```console
  vim build/images/nri-resmgr-topology-aware-deployment.yaml
  ```

- Then deploy the plugin simlarly to the earlier step.

## Migrating existing configuration

- The ConfigMap used by the ported policies/infra has a different name/naming scheme than the original one used in CRI-RM, ex:
  - configMapName:
    ```diff
    - configmap-name: cri-resmgr-config
    + configmap-name: nri-resource-policy-config
    ```
  - The details of grouping nodes by labeling to share configuration:
    ```diff
    - cri-resource-manager.intel.com/group: $GROUP_NAME
    + resource-policy.nri.io/group: $GROUP_NAME
    ```

## Migrating existing workloads

- The annotations one can use to customize how a policy treats a workload use slightly different keys than the original ones in CRI-RM. The collective 'key namespace' for policy- and resource-manager-specific annotation has been changed from cri-resource-manager.intel.com to resource-policy.nri.io.

- For instance, an explicit type annotation for the balloons policy, which used to be:
  ```yaml
  ...
  metadata:
    annotations:
      balloon.balloons.cri-resource-manager.intel.com/container.$CONTAINER_NAME: $BALLOON_TYPE`
  ...
  ```

- Should now be:
  ```yaml
  ...
  metadata:
    annotations:
      balloon.balloons.resource-policy.nri.io/container.$CONTAINER_NAME: $BALLOON_TYPE`
  ...
  ```

- Similarly a workload opt-out annotation from exclusive CPU allocation for the topology-aware policy, which used to be:
  ```yaml
  ...
  metadata:
    annotations:
      prefer-shared-cpus.cri-resource-manager.intel.com/container.$CONTAINER_NAME: "true"
  ...
  ```

- Should now be:
  ```yaml
  ...
  metadata:
    annotations:
      prefer-shared-cpus.resource-policy.nri.io/container.$CONTAINER_NAME: "true"
  ...
  ```

- Similar changes are needed for any cri-resmgr-specific annotation that uses the same semantic scoping for key syntax.


All of the annotations:
| Was                                                 | Is now                                      |
| --------------------------------------------------- | ------------------------------------------- |
| cri-resource-manager.intel.com/afffinity            | resource-policy.nri.io/affinity             |
| cri-resource-manager.intel.com/anti-afffinity       | resource-policy.nri.io/anti-affinity        |
| cri-resource-manager.intel.com/prefer-isolated-cpus | resource-policy.nri.io/prefer-isolated-cpus |
| cri-resource-manager.intel.com/prefer-shared-cpus   | resource-policy.nri.io/prefer-shared-cpus   |
| cri-resource-manager.intel.com/cold-start           | resource-policy.nri.io/cold-start           |
| cri-resource-manager.intel.com/memory-type          | resource-policy.nri.io/ memory-type         |
| prefer-isolated-cpus.cri-resource-manager.intel.com | prefer-isolated-cpus.resource-policy.nri.io |
| prefer-shared-cpus.cri-resource-manager.intel.com   | prefer-shared-cpus.resource-policy.nri.io   |
| memory-type.cri-resource-manager.intel.com          | memory-type.resource-policy.nri.io          |
| cold-start.cri-resource-manager.intel.com           | cold-start.resource-policy.nri.io           |
| prefer-reserved-cpus.cri-resource-manager.intel.com | prefer-reserved-cpus.resource-policy.nri.io |
| rdtclass.cri-resource-manager.intel.com             | rdtclass.resource-policy.nri.io             |
| blockioclass.cri-resource-manager.intel.com         | blockioclass.resource-policy.nri.io         |
| toptierlimit.cri-resource-manager.intel.com         | toptierlimit.resource-policy.nri.io         |
| topologyhints.cri-resource-manager.intel.com        | topologyhints.resource-policy.nri.io        |
| balloon.balloons.cri-resource-manager.intel.com     | balloon.balloons.resource-policy.nri.io     |
