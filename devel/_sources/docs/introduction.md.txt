# Introduction

CRI Resource Manager is a Container Runtime Interface Proxy. It sits between
clients and the actual Container Runtime implementation (containerd, cri-o)
relaying requests and responses back and forth. The main purpose of the proxy
is to apply hardware-aware resource allocation policies to the containers
running in the system.

Policies are applied by either modifying a request before forwarding it or
by performing extra actions related to the request during its processing and
proxying. There are several policies available, each with a different set of
goals in mind and implementing different hardware allocation strategies. The
details of whether and how a CRI request is altered or if extra actions are
performed depend on which policy is active in CRI Resource Manager and how
that policy is configured.

The current goal for the CRI Resource Manager is to prototype and experiment
with new Kubernetes\* container placement policies. The existing policies are
written with this in mind and the intended setup is for the Resource Manager
to only act as a proxy for the Kubernetes Node Agent, kubelet.
