# Container Affinity and Anti-Affinity

## Introduction

Some policies allow the user to give hints about how particular containers
should be *co-located* within a node. In particular these hints express whether
containers should be located *'close'* to each other or *'far away'* from each
other, in a hardware topology sense.

Since these hints are interpreted always by a particular *policy implementation*,
the exact definitions of 'close' and 'far' are also somewhat *policy-specific*.
However as a general rule of thumb containers running

  - on CPUs within the *same NUMA nodes* are considered *'close'* to each other,
  - on CPUs within *different NUMA nodes* in the *same socket* are *'farther'*, and
  - on CPUs within *different sockets* are *'far'* from each other

These hints are expressed by `container affinity annotations` on the Pod.
There are two types of affinities:

  - `affinity` (or `positive affinty`): cause affected containers to *pull* each other closer
  - `anti-affinity` (or `negative affinity`): cause affected containers to *push* each other further away

Policies try to place a container
  - close to those the container has affinity towards
  - far from those the container has anti-affinity towards.

## Affinity Annotation Syntax

*Affinities* are defined as the `cri-resource-manager.intel.com/affinity` annotation.
*Anti-affinities* are defined as the `cri-resource-manager.intel.com/anti-affinity`
annotation. They are specified in the `metadata` section of the `Pod YAML`, under
`annotations` as a dictionary, with each dictionary key being the name of the
*container* within the Pod to which the annotation belongs to.

```yaml
metadata:
  anotations:
    cri-resource-manager.intel.com/affinity: |
      container1:
        - scope:
            key: [optional-key-domain/]key
            operator: op
            values:
            - value1
            ...
            - valueN
        - match:
            key: [optional-key-domain/]key
            operator: op
            values:
            - value1
            ...
            - valueN
          weight: w
```

An anti-affinity is defined similarly but using `cri-resource-manager.intel.com/anti-affinity`
as the annotation key.

```yaml
metadata:
  anotations:
    cri-resource-manager.intel.com/anti-affinity: |
      container1:
        - scope:
            key: [optional-key-domain/]key
            operator: op
            values:
            - value1
            ...
            - valueN
        - match:
            key: [optional-key-domain/]key
            operator: op
            values:
            - value1
            ...
            - valueN
          weight: w
```

## Affinity Semantics

An affinity consists of three parts:

  - `scope expression`: defines which containers this affinity is evaluated against
  - `match expression`: defines for which containers (within the scope) the affinity applies to
  - `weight`: defines how *strong* a pull or a push the affinity causes

*Affinities* are also sometimes referred to as *positive affinities* while
*anti-affinities* are referred to as *negative affinities*. The reason for this is
that the only difference between these are that affinities have a *positive weight*
while anti-affinities have a *negative weight*.

The *scope* of an affinity defines the *bounding set of containers* the affinity can
apply to. The affinity *expression* is evaluated against the containers *in scope* and
it *selects the containers* the affinity really has an effect on. The *weight* specifies
whether the effect is a *pull* or a *push*. *Positive* weights cause a *pull* while
*negative* weights cause a *push*. Additionally, the *weight* specifies *how strong* the
push or the pull is. This is useful in situations where the policy needs to make some
compromises because an optimal placement is not possible. The weight then also acts as
a way to specify preferences of priorities between the various compromises: the heavier
the weight the stronger the pull or push and the larger the propbability that it will be
honored, if this is possible at all.

The scope can be omitted from an affinity in which case it implies *Pod scope*, in other
words the scope of all containers that belong to the same Pod as the container for which
which the affinity is defined.

The weight can also be omitted in which case it defaults to -1 for anti-affinities
and +1 for affinities.

Both the affinity scope and the expression select containers, therefore they are identical.
Both of them are *expressions*. An expression consists of three parts:

  - key: specifies what *metadata* to pick from a container for evaluation
  - operation (op): specifies what *logical operation* the expression evaluates
  - values: a set of *strings* to evaluate the the value of the key against

Essentially an expression defines a logical operation of the form (key op values).
Evaluating this logical expression will take the value of the key in  which
either evaluates to true or false. 
a boolean true/false result. Currently the following operations are supported:

  - `Equals`: equality, true if the *value of key* equals the single item in *values*
  - `NotEqual`: inequality, true if the *value of key* is not equal to the single item in *values*
  - `In`: membership, true if *value of key* equals to any among *values*
  - `NotIn`: negated membership, true if the *value of key* is not equal to any among *values*
  - `Exists`: true if the given *key* exists with any value
  - `NotExists`: true if the given *key* does not exist
  - `AlwaysTrue`: always evaluates to true, can be used to denote node-global scope (all containers)

The effective affinity between containers C_1 and C_2, A(C_1, C_2) is the sum of the
weights of all pairwise in-scope matching affinities W(C_1, C_2). To put it another way,
evaluating an affinity for a container C_1 is done by first using the scope (expression)
to determine which containers are in the scope of the affinity. Then, for each in-scope
container C_2 for which the match expression evaluates to true, taking the weight of the
affinity and adding it to the effective affinity A(C_1, C_2).

Note that currently (for the topology-aware policy) this evaluation is asymmetric:
A(C_1, C_2) and A(C_2, C_1) can and will be different unless the affinity annotations are
crafted to prevent this (by making them fully symmetric). Moreover, A(C_1, C_2) is calculated
and taken into consideration during resource allocation for C_1, while A(C_2, C_1)
is calculated and taken into account during resource allocation for C_2. This might be
changed in a future version.


## Examples

Put the container `peter` close to the container `sheep` but far away from the
container `wolf`.

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |
      peter:
      - match:
          key: name
          operator: Equals
          values:
          - sheep
        weight: 5
    cri-resource-manager.intel.com/anti-affinity: |
      peter:
      - match:
          key: name
          operator: Equals
          values:
          - wolf
        weight: 5
```

## Shorthand Notation

There is an alternative shorthand syntax for what is considered to be the most common
case: defining affinities between containers within the same pod. With this notation
one needs to give just the names of the containers, like in the example below.

```yaml
  annotations:
    cri-resource-manager.intel.com/affinity: |
      container3: [ container1 ]
    cri-resource-manager.intel.com/anti-affinity: |
      container3: [ container2 ]
      container4: [ container2, container3 ]
```


This shorthand notation defines:
  - `container3` having
    - affinity (weight 1) to `container1`
    - `anti-affinity` (weight -1) to `container2`
  - `container4` having
    - `anti-affinity` (weight -1) to `container2`, and `container3`

The equivalent annotation in full syntax would be

```yaml
metadata:
  annotations:
    cri-resource-manager.intel.com/affinity: |+
      container3:
      - match:
          key: labels/io.kubernetes.container.name
          operator: In
          values:
          - container1
    cri-resource-manager.intel.com/anti-affinity: |+
      container3:
      - match:
          key: labels/io.kubernetes.container.name
          operator: In
          values:
          - container2
      container4:
      - match:
          key: labels/io.kubernetes.container.name
          operator: In
          values:
          - container2
          - container3
```
