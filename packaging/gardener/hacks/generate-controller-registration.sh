chart="$(tar -C charts -c gardener-extension-cri-rm | gzip -n | base64 | tr -d '\n')"
OUT=examples/ctrldeploy-ctrlreg.yaml

#FOR DEBUG
#rm -rf /tmp/extract_dir && mkdir -p /tmp/extract_dir/ ; echo $chart | base64 -d  | gunzip | tar -xv -C /tmp/extract_dir && find /tmp/extract_dir

cat <<EOT > "$OUT"
---
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerDeployment
metadata:
  name: cri-rm-extension
type: helm
providerConfig:
  chart: $chart
---
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerRegistration
metadata:
  name: cri-rm-extension
spec:
  deployment:
    deploymentRefs:
    - name: cri-rm-extension
  resources:
  - kind: Extension
    type: cri-rm-extension
    globallyEnabled: true
EOT

echo "Successfully generated ControllerRegistration and ControllerDeployment example."
