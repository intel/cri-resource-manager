#!/bin/sh -e

this=`realpath "$0"`
this_dir=`dirname "$this"`
template_dir=`realpath "$this_dir/../../cmd/webhook/"`
outdir="deploy/cri-resmgr-webhook"
outdir_abs="`pwd`/$outdir"

cat << EOF
***                                 ***
*** WARNING: NOT FOR PRODUCTION USE ***
***                                 ***

EOF

info () {
    echo "[INFO] $1"
}

info "Generating x509 keys..."

mkdir -p "$outdir"

# Create temp workdir and remove it on exit
tmpdir=`mktemp -d --suffix=.cri-resmgr`
trap "rm -rf '$tmpdir'" EXIT

cd $tmpdir

# Create a self-signed CA certificate
openssl req -batch -new -newkey rsa:2048 -x509 -sha256 -nodes -days=30 -out ca.crt -keyout ca.key

export cn=cri-resmgr-webhook.cri-resmgr.svc
openssl req -batch -newkey rsa:2048 -nodes -keyout svc.key -out $cn.csr -subj "/CN=cri-resmgr-webhook.cri-resmgr.svc"
openssl x509 -req -in $cn.csr -CA ca.crt  -CAkey ca.key -CAcreateserial -sha256 -out svc.crt -days 3650

# Copy artifacts to outdir
cp ca.crt svc.crt svc.key "$outdir_abs"

info "Done"
info "Sample cert and key files successfully generated under '$outdir'"

info "Creating MutatingWebhookConfiguration template"
sed s"/CA_BUNDLE_PLACEHOLDER/`cat ca.crt | base64 -w0`/" "$template_dir/mutating-webhook-config.yaml" > "$outdir_abs/mutating-webhook-config.yaml"

# Print instructions
cat << EOF

Instructions for example deployment
===================================
0. Create cri-resmgr namespace, if it does not exist:
   kubectl create ns cri-resmgr

1. Create Kubernetes secrets with:
   kubectl -n cri-resmgr create secret generic cri-resmgr-webhook-secret \\
    --from-file=$outdir/svc.crt --from-file=$outdir/svc.key

2. Build and publish webhook container:
   make image-webhook IMAGE_REPO=my-image-repo IMAGE_TAG=my-version

   And deploy it:
   sed s'!IMAGE_PLACEHOLDER!my-image-repo/cri-resmgr-webhook:my-version!' cmd/webhook/webhook-deployment.yaml | kubectl apply -f -

3. Create MutatingWebhookConfiguration with:
   kubectl apply -f $outdir/mutating-webhook-config.yaml

EOF
