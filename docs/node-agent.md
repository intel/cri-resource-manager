# Node Agent

CRI Resource Manager can be configured dynamically using the CRI Resource
Manager Node Agent and Kubernetes ConfigMaps. The agent can be build using
the [provided Dockerfile](/cmd/cri-resmgr-agent/Dockerfile). It can be deployed
as a `DaemonSet` in the cluster using the [provided deployment file](/cmd/cri-resmgr-agent/agent-deployment.yaml).

To run the agent manually or as a `systemd` service, set the environment variable
`NODE_NAME` to the name of the cluster node the agent is running on. If necessary
pass it the credentials for accessing the cluster using the the `-kubeconfig <file>`
command line option.

The agent monitors two ConfigMaps for the node, a primary node-specific one, and
a secondary group-specific or default one, depending on whether the node belongs
to a configuration group. The node-specific ConfigMap always takes precedence over
the others.

The names of these ConfigMaps are

1. `cri-resmgr-config.node.$NODE_NAME`: primary, node-specific configuration
2. `cri-resmgr-config.group.$GROUP_NAME`: secondary group-specific node configuration
3. `cri-resmgr-config.default`: secondary: secondary default node configuration

You can assign a node to a configuration group by setting the
`cri-resource-manager.intel.com/group` label on the node to the name of
the configuration group. You can remove a node from its group by deleting the node
group label.

There is a [sample ConfigMap spec](/sample-configs/cri-resmgr-configmap.example.yaml)
that contains anode-specific, a group-specific, and a default ConfigMap examples.
See [any available policy-specific documentation](docs) for more information on the
policy configurations.


