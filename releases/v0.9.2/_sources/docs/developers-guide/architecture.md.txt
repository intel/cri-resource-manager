# Architecture

## Overview

CRI Resource Manager (CRI-RM) is a pluggable add-on for controlling how much
and which resources are assigned to containers in a Kubernetes\* cluster.
It's an add-on because you install it in addition to the normal selection of
your components. It's pluggable since you inject it on the signaling path
between two existing components with the rest of the cluster unaware of its
presence.

CRI-RM plugs in between kubelet and CRI, the Kubernetes node agent and the
container runtime implementation. CRI-RM intercepts CRI protocol requests
from the kubelet acting as a non-transparent proxy towards the runtime.
Proxying by CRI-RM is non-transparent in nature because it usually alters
intercepted protocol messages before forwarding them.

CRI-RM keeps track of the states of all containers running on a Kubernetes
node. Whenever it intercepts a CRI request that results in changes to the
resource allocation of any container (container creation, deletion, or
resource assignment update request), CRI-RM runs one of its built-in policy
algorithms. This policy makes a decision about how the assignment of
resources should be updated and, eventually, the intercepted request is
modified according to this decision. The policy can make changes to any
container in the system, not just the one associated with the intercepted
CRI request. Therefore it does not operate directly on CRI requests.
Instead CRI-RM's internal state tracking cache provides an abstraction for
modifying containers and the policy uses this abstraction for recording its
decisions.

In addition to policies, CRI-RM has a number of built-in resource
controllers.
These are used to put policy decisions—in practice pending changes made to
containers by a policy—into effect. A special in-band CRI controller is used
to control all resources that are controllable via the CRI runtime. This
controller handles the practical details of updating the intercepted CRI
request and generating any additional unsolicited update requests for other
existing containers updated by the policy decision. Additional out-of-band
controllers exist to exercise control over resources that the current CRI
runtimes are unable to handle.

To tell which containers need to be handed off to various controllers for
updating, CRI-RM uses the internal state tracking cache's ability to tell
which containers have pending unenforced changes and to which controllers'
domain these changes belong. The CRI controller currently handles CPU and
memory resources, including huge pages. The level of control covers per
container CPU sets, CFS parametrization, memory limits, OOM score adjustment,
and pinning to memory controllers. The two existing out-of-band controllers,
Intel® Resource Director Technology (Intel® RDT) and Block I/O, handle last-level cache and memory bandwidth allocation,
and the arbitration of Block I/O bandwidth respectively.

Many of the details of how CRI-RM operates is configurable. These include,
for instance, which policy is active within CRI-RM, configuration of the
resource assignment algorithm for the active policy, and configuration for
the various resource controllers. Although CRI-RM can be configured using a
configuration file present on the node running CRI-RM, the preferred way to
configure all CRI-RM instances in a cluster is to use Kubernetes
ConfigMaps and the CRI-RM Node Agent.

<p align="center">
<!-- # It's a pity the markdown ![]()-syntax does not support aligning... -->
<img src="figures/arch-overview.svg" title="Architecture Overview">
</p>

## Components

### [Node Agent](/pkg/agent/)

The node agent is a component external to CRI-RM itself. All interactions
by CRI-RM with the Kubernetes Control Plane go through the node agent with
the node agent performing any direct interactions on behalf of CRI-RM.

The node agent communicates with CRI-RM using two gRPC interfaces. The
[config interface](/pkg/cri/resource-manager/config/api/v1/) is used to:
  - push updated external configuration data to CRI-RM
  - push adjustments to container resource assignments to CRI-RM

The [cluster interface](/pkg/agent/api/v1/) implements the necessary
low-level plumbing for the agent interface CRI-RM internally exposes
for its policies and other components. This interface in turn implements
the following:
  - updating resource capacity of the node
  - getting, setting, or removing labels on the node
  - getting, setting, or removing annotations on the node
  - getting, setting, or removing taints on the node

The config interface is defined and has its gRPC server running in
CRI-RM. The agent acts as a gRPC client for this interface. The low-level
cluster interface is defined and has its gRPC server running in the agent,
with the [convenience layer](/pkg/cri/resource-manager/agent) defined in
CRI-RM. CRI-RM acts as a gRPC client for the low-level plumbing interface.

Additionally, the stock node agent that comes with CRI-RM implements schemes
for:
   - configuration management for all CRI-RM instances
   - management of dynamic adjustments to container resource assignments

<p align="center">
<!-- # It's a pity the markdown ![]()-syntax does not support aligning... -->
<img src="figures/cri-resmgr.png" title="Architecture Overview" width="50%">
</p>


### [Resource Manager](/pkg/cri/resource-manager/)

CRI-RM implements a request processing pipeline and an event processing
pipeline.
The request processing pipeline takes care of proxying CRI requests and
responses between CRI clients and the CRI runtime. The event processing
pipeline processes a set of other events that are not directly related
to or the result of CRI requests. These events are typically internally
generated within CRI-RM. They can be the result of changes in the state
of some containers or the utilization of a shared system resource, which
potentially could warrant an attempt to rebalance the distribution of
resources among containers to bring the system closer to an optimal state.
Some events can also be generated by policies.

The Resource Manager component of CRI-RM implements the basic control
flow of both of these processing pipelines. It passes control to all the
necessary sub-components of CRI-RM at the various phases of processing a
request or an event. Additionally, it serializes the processing of these,
making sure there is at most one (intercepted) request or event being
processed at any point in time.

The high-level control flow of the request processing pipeline is as
follows:

A. If the request does not need policying, let it bypass the processing
pipeline; hand it off for logging, then relay it to the server and the
corresponding response back to the client.

B. If the request needs to be intercepted for policying, do the following:
 1. Lock the processing pipeline serialization lock.
 2. Look up/create cache objects (pod/container) for the request.
 3. If the request has no resource allocation consequences, do proxying
    (step 6).
 4. Otherwise, invoke the policy layer for resource allocation:
    - Pass it on to the configured active policy, which will
    - Allocate resources for the container.
    - Update the assignments for the container in the cache.
    - Update any other containers affected by the allocation in the cache.
 5. Invoke the controller layer for post-policy processing, which will:
    - Collect controllers with pending changes in their domain of control
    - for each invoke the post-policy processing function corresponding to
      the request.
    - Clear pending markers for the controllers.
 6. Proxy the request:
    - Relay the request to the server.
    - Send update requests for any additional affected containers.
    - Update the cache if/as necessary based on the response.
    - Relay the response back to the client.
 7. Release the processing pipeline serialization lock.

The high-level control flow of the event processing pipeline is one of the
following, based on the event type:

 - For policy-specific events:
   1. Engage the processing pipeline lock.
   2. Call policy event handler.
   3. Invoke the controller layer for post-policy processing (same as step 5 for requests).
   4. Release the pipeline lock.
 - For metrics events:
   1. Perform collection/processing/correlation.
   2. Engage the processing pipeline lock.
   3. Update cache objects as/if necessary.
   4. Request rebalancing as/if necessary.
   5. Release pipeline lock.
 - For rebalance events:
   1. Engage the processing pipeline lock.
   2. Invoke policy layer for rebalancing.
   3. Invoke the controller layer for post-policy processing (same as step 5 for requests).
   4. Release the pipeline lock.


### [Cache](/pkg/cri/resource-manager/cache/)

The cache is a shared internal storage location within CRI-RM. It tracks the
runtime state of pods and containers known to CRI-RM, as well as the state
of CRI-RM itself, including the active configuration and the state of the
active policy. The cache is saved to permanent storage in the filesystem and
is used to restore the runtime state of CRI-RM across restarts.

The cache provides functions for querying and updating the state of pods and
containers. This is the mechanism used by the active policy to make resource
assignment decisions. The policy simply updates the state of the affected
containers in the cache according to the decisions.

The cache's ability to associate and track changes to containers with
resource domains is used to enforce policy decisions. The generic controller
layer first queries which containers have pending changes, then invokes each
controller for each container. The controllers use the querying functions
provided by the cache to decide if anything in their resource/control domain
needs to be changed and then act accordingly.

Access to the cache needs to be serialized. However, this serialization is
not provided by the cache itself. Instead, it assumes callers to make sure
proper protection is in place against concurrent read-write access. The
request and event processing pipelines in the resource manager use a lock to
serialize request and event processing and consequently access to the cache.

If a policy needs to do processing unsolicited by the resource manager, IOW
processing other than handling the internal policy backend API calls from the
resource manager, then it should inject a policy event into the resource
managers event loop. This causes a callback from the resource manager to
the policy's event handler with the injected event as an argument and with
the cache properly locked.


### [Generic Policy Layer](/pkg/cri/resource-manager/policy/policy.go)

The generic policy layer defines the abstract interface the rest of CRI-RM
uses to interact with policy implementations and takes care of the details
of activating and dispatching calls through to the configured active policy.


### [Generic Resource Controller Layer](/pkg/cri/resource-manager/control/control.go)

The generic resource controller layer defines the abstract interface the rest
of CRI-RM uses to interact with resource controller implementations and takes
care of the details of dispatching calls to the controller implementations
for post-policy enforcment of decisions.


### [Metrics Collector](/pkg/metrics/)

The metrics collector gathers a set of runtime metrics about the containers
running on the node. CRI-RM can be configured to periodically evaluate this
collected data to determine how optimal the current assignment of container
resources is and to attempt a rebalancing/reallocation if it is deemed
both possible and necessary.


### [Policy Implementations](/pkg/cri/resource-manager/policy/builtin/)

#### [None](/pkg/cri/resource-manager/policy/builtin/none/)

An empty policy that makes no policy decisions. It is included
merely for the sake of completeness, analoguous to the none policy of the
CPU Manager in kubelet.

#### [Static Pools](/pkg/cri/resource-manager/policy/builtin/static-pools/)

A backward-compatible reimplementation of
[CMK](https://github.com/intel/CPU-Manager-for-Kubernetes)
for CRI-RM with a few extra features.

#### [Static](/pkg/cri/resource-manager/policy/builtin/static/)

Part of the code from the static policy of CPU Manager in kubelet, that has
been brutally hacked to work within CRI-RM. Serves merely as a
proof-of-concept that the current policies of kubelet can be implemented in
CRI-RM.

#### [Static Plus](/pkg/cri/resource-manager/policy/builtin/static-plus/)

A fairly simplistic policy similar in spirit to the static policy of
CPU Manager in kubelet, with a few extra features.

#### [Topology Aware](/pkg/cri/resource-manager/policy/builtin/topology-aware/)

A topology-aware policy capable of handling multiple tiers/types of memory,
typically a DRAM/PMEM combination configured in 2-layer memory mode.

### [Resource Controller Implementations](/pkg/cri/resource-manager/control/)

#### [Intel RDT](/pkg/cri/resource-manager/control/rdt/)

A resource controller implementation responsible for the practical details of
associating a container with Intel RDT classes. This class effectively
determines how much last level cache and memory bandwidth will be available
for the container. This controller uses the resctrl pseudo filesystem of the
Linux kernel for control.

#### [Block I/O](/pkg/cri/resource-manager/control/blockio/)

A resource controller implementation responsible for the practical details of
associating a container with a Block I/O class. This class effectively
determines how much Block I/O bandwidth will be available for the container.
This controller uses the blkio cgroup controller and the cgroupfs pseudo-
filesystem of the Linux kernel for control.

#### [CRI](/pkg/cri/resource-manager/control/cri/)

A resource controller responsible for modifying intercepted CRI container
creation requests and creating CRI container resource update requests,
according to the changes the active policy makes to containers.
