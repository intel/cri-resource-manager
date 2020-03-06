module github.com/intel/cri-resource-manager

go 1.13

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.1-0.20191218042359-6151c48ac7fa
	github.com/ghodss/yaml v1.0.0
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.3.1
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0
	github.com/iovisor/gobpf v0.0.0-20191024162143-7c8f8e040b4b
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.2.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.7.0
	go.opencensus.io v0.22.2
	golang.org/x/net v0.0.0-20190812203447-cdfb69ac37fc
	golang.org/x/sys v0.0.0-20200202164722-d101bd2416d5
	golang.org/x/time v0.0.0-20161028155119-f51c12702a4d
	google.golang.org/grpc v1.20.1
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/cri-api v0.0.0
	k8s.io/kubernetes v1.15.3
)

// versions are based on k8s.io/kubernetes
replace (
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0 => ./pkg/topology
	k8s.io/api v0.0.0 => k8s.io/api v0.0.0-20190819141258-3544db3b9e44
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.0.0-20190819143637-0dbe462fe92d
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/apiserver v0.0.0 => k8s.io/apiserver v0.0.0-20190819142446-92cc630367d0
	k8s.io/cli-runtime v0.0.0 => k8s.io/cli-runtime v0.0.0-20190819144027-541433d7ce35
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.0.0-20190819141724-e14f31a72a77
	k8s.io/cloud-provider v0.0.0 => k8s.io/cloud-provider v0.0.0-20190819145148-d91c85d212d5
	k8s.io/cluster-bootstrap v0.0.0 => k8s.io/cluster-bootstrap v0.0.0-20190819145008-029dd04813af
	k8s.io/code-generator v0.0.0 => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
	k8s.io/component-base v0.0.0 => k8s.io/component-base v0.0.0-20190819141909-f0f7c184477d
	k8s.io/cri-api v0.0.0 => k8s.io/cri-api v0.0.0-20190817025403-3ae76f584e79
	k8s.io/csi-translation-lib v0.0.0 => k8s.io/csi-translation-lib v0.0.0-20190819145328-4831a4ced492
	k8s.io/kube-aggregator v0.0.0 => k8s.io/kube-aggregator v0.0.0-20190819142756-13daafd3604f
	k8s.io/kube-controller-manager v0.0.0 => k8s.io/kube-controller-manager v0.0.0-20190819144832-f53437941eef
	k8s.io/kube-proxy v0.0.0 => k8s.io/kube-proxy v0.0.0-20190819144346-2e47de1df0f0
	k8s.io/kube-scheduler v0.0.0 => k8s.io/kube-scheduler v0.0.0-20190819144657-d1a724e0828e
	k8s.io/kubectl v0.0.0 => k8s.io/kubectl v0.0.0-20190602132728-7075c07e78bf
	k8s.io/kubelet v0.0.0 => k8s.io/kubelet v0.0.0-20190819144524-827174bad5e8
	k8s.io/legacy-cloud-providers v0.0.0 => k8s.io/legacy-cloud-providers v0.0.0-20190819145509-592c9a46fd00
	k8s.io/metrics v0.0.0 => k8s.io/metrics v0.0.0-20190819143841-305e1cef1ab1
	k8s.io/sample-apiserver v0.0.0 => k8s.io/sample-apiserver v0.0.0-20190819143045-c84c31c165c4
)
