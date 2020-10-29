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
the [provided Dockerfile](/cmd/cri-resmgr-webhook/Dockerfile) and published it,
you can set up the webhook as follows:
- Fill in the `IMAGE_PLACEHOLDER` in [webhook-deployment.yaml](/cmd/cri-resmgr-webhook/webhook-deployment.yaml) to match the image.
- Create a `cri-resmgr-webhook-secret` that carries a key and a certificate to `cri-resmgr-webhook`. You can create a key, a self-signed certificate and the secret that holds them with commands:
  ```bash
  SVC=cri-resmgr-webhook NS=cri-resmgr
  openssl req -x509 -newkey rsa:2048 -sha256 -days 365 -nodes \
    -keyout cmd/cri-resmgr-webhook/server-key.pem \
    -out cmd/cri-resmgr-webhook/server-crt.pem \
    -subj "/CN=$SVC.$NS.svc" \
    -addext "subjectAltName=DNS:$SVC,DNS:$SVC.$NS,DNS:$SVC.$NS.svc"
  cat >cmd/cri-resmgr-webhook/webhook-secret.yaml <<EOF
  apiVersion: v1
  kind: Secret
  metadata:
    name: cri-resmgr-webhook-secret
    namespace: $NS
  data:
    svc.crt: $(base64 -w0 < cmd/cri-resmgr-webhook/server-crt.pem)
    svc.key: $(base64 -w0 < cmd/cri-resmgr-webhook/server-key.pem)
  type: Opaque
  EOF
  kubectl create namespace $NS
  kubectl create -f cmd/cri-resmgr-webhook/webhook-secret.yaml
  ```
- Fill in the `CA_BUNDLE_PLACEHOLDER` in [mutating-webhook-config.yaml](/cmd/cri-resmgr-webhook/mutating-webhook-config.yaml).
  If you created the key and the certificate with the commands above,
  you can do this with command:
  ```bash
  sed -e "s/CA_BUNDLE_PLACEHOLDER/$(base64 -w0 < cmd/cri-resmgr-webhook/server-crt.pem)/" \
      -i cmd/cri-resmgr-webhook/mutating-webhook-config.yaml
  ```
- Finally set up the webhook with these commands:
  ```bash
  kubectl apply -f cmd/cri-resmgr-webhook/webhook-deployment.yaml
  kubectl wait --for=condition=Available -n cri-resmgr deployments/cri-resmgr-webhook
  kubectl apply -f cmd/cri-resmgr-webhook/mutating-webhook-config.yaml
  ```
