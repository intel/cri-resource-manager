# Webhook

By default CRI Resource Manager does not see the original container *resource
requirements* specified in the *Pod Spec*. It tries to calculate these for `cpu`
and `memory` *compute resource*s using the related parameters present in the
CRI container creation request. The resulting estimates are normally accurate
for `cpu`, and also for `memory` `limits`. However, it is not possible to use
these parameters to estimate `memory` `request`s or any *extended resource*s.

If you want to make sure that CRI Resource Manager uses the origin *Pod Spec*
*resource requirement*s, you need to duplicate these as *annotations* on the Pod.
This is necessary if you plan using or writing a policy which needs *extended
resource*s.

This process can be fully automated using the [CRI Resource Manager Annotating
Webhook](/cmd/cri-resmgr-webhook). Once you built the docker image for it using
the [provided Dockerfile][/cmd/cri-resmgr-webhook/Dockerfile] and published it,
you can set up the webhook with these commands:

```
  kubectl apply -f cmd/cri-resmgr-webhook/mutating-webhook-config.yaml
  kubectl apply -f cmd/cri-resmgr-webhook/webhook-deployment.yaml

```

