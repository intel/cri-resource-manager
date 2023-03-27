# Node Agent

CRI Resource Manager can be configured dynamically using the CRI Resource
Manager Node Agent and Kubernetes\* ConfigMaps.

## Running as a DaemonSet

The agent can be build using the
[provided Dockerfile](/cmd/cri-resmgr-agent/Dockerfile). It can be
deployed as a `DaemonSet` in the cluster using the
[provided deployment file](/cmd/cri-resmgr-agent/agent-deployment.yaml).

When using the provided or a similar deployment, the agent uses a
readiness probe to propagate the status of the last configuration
update back to the control plane. If the configuration could not
be taken into use for any reason, the agent's probe will fail which
eventually marks the agent as not being `Ready`. In this case, more
details about the failure should be present among the latest messages
logged by the agent or the probe itself. if the reason for failure is
a configuration error, once the error is fixed, the agent should become
eventually `Ready` again.

## Running as a Host Service

To run the agent manually or as a `systemd` service, set the environment
variable `NODE_NAME` to the name of the cluster node the agent is running
on. If necessary pass it the credentials for accessing the cluster using
 the `-kubeconfig <file>` command line option.

## ConfigMap to Node Mapping Conventions

The agent monitors two ConfigMaps for the node, a primary node-specific one
and a secondary group-specific or default one, depending on whether the node
belongs to a configuration group. The node-specific ConfigMap always takes
precedence over the others.

The names of these ConfigMaps are

1. `cri-resmgr-config.node.$NODE_NAME`: primary, node-specific configuration
2. `cri-resmgr-config.group.$GROUP_NAME`: secondary group-specific node
    configuration
3. `cri-resmgr-config.default`: secondary: secondary default node
    configuration

You can assign a node to a configuration group by setting the
`cri-resource-manager.intel.com/group` label on the node to the name of
the configuration group. You can remove a node from its group by deleting
the node group label.

There is a
[sample ConfigMap spec](/sample-configs/cri-resmgr-configmap.example.yaml)
that contains a node-specific, a group-specific, and a default ConfigMap
example. See [any available policy-specific documentation](policy/index.rst)
for more information on the policy configurations.

