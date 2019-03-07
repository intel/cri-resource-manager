# TODO for CRI Resource Manager

## Builtin Policies

### Static-Pools
- helper tool for creating configuration (á la `cmk init`)

## Misc.

### Multiple Active Policies
- cache: add support for saving data for multiple policies
- policy: pick policy for container creation requests

### RDT
- re-add RDT support in a policy-agnostic manner
- RDT-specific pieces: CLoS creation/configuration, mechanism for enforcement
- post-start hook: pick RDT CLoS tag attached by policy, enforce CLoS class
- (some) policies: tag container with desired RDT CLoS name

### Policy:
- define/pass explicitly interfaces for commonly needed functionality to policies,
at least for
  - system introspection (sysfs, topology, etc.)
  - CPU allocation (with customizable core preference calculation/sorting)

## Cleanups
- needed all over the place

## Testing
- needed all over the place, only `static-pools` has test cases
- end goal is to be able to automatically test the policy decision making
in complete isolation by feeding synthetic requests and checking the resulting
decisions/actions are as expected
