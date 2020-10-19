module github.com/intel/cri-resource-manager

go 1.14

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.1-0.20191218042359-6151c48ac7fa
	github.com/cilium/ebpf v0.0.0-20200317182658-6f632cc9ee38
	github.com/evanphx/json-patch v4.2.0+incompatible
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.4.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0
	github.com/intel/goresctrl v0.0.0-20201123171612-0d2cbe434ecd
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.8.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.14.0
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	go.opencensus.io v0.22.2
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/sys v0.0.0-20201015000850-e3ed0017c211
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	google.golang.org/grpc v1.26.0
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/cri-api v0.0.0
	k8s.io/klog/v2 v2.4.0
	k8s.io/kubernetes v1.17.2
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0 => ./pkg/topology
	google.golang.org/grpc => google.golang.org/grpc v1.26.0

	k8s.io/api v0.0.0 => k8s.io/api v0.0.0-20200121193204-7ea599edc7fd
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.0.0-20200121201129-111e9ba415da
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.0.0-20191121175448-79c2a76c473a
	k8s.io/apiserver v0.0.0 => k8s.io/apiserver v0.0.0-20200121195158-da2f3bd69287
	k8s.io/cli-runtime v0.0.0 => k8s.io/cli-runtime v0.0.0-20200121201805-7928b415bdea
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.0.0-20200121193945-bdedab45d4f6
	k8s.io/cloud-provider v0.0.0 => k8s.io/cloud-provider v0.0.0-20200121203829-580c13bb6ed9
	k8s.io/cluster-bootstrap v0.0.0 => k8s.io/cluster-bootstrap v0.0.0-20200121203528-48c15d793bf4
	k8s.io/code-generator v0.0.0 => k8s.io/code-generator v0.0.0-20191121175249-e95606b614f0
	k8s.io/component-base v0.0.0 => k8s.io/component-base v0.0.0-20200121194253-47d744dd27ec
	k8s.io/cri-api v0.0.0 => k8s.io/cri-api v0.0.0-20191121183020-775aa3c1cf73
	k8s.io/csi-translation-lib v0.0.0 => k8s.io/csi-translation-lib v0.0.0-20200121204128-ab1d1be7e7e9
	k8s.io/kube-aggregator v0.0.0 => k8s.io/kube-aggregator v0.0.0-20200121195706-c8017da6deb7
	k8s.io/kube-controller-manager v0.0.0 => k8s.io/kube-controller-manager v0.0.0-20200121203241-7fc8a284e25f
	k8s.io/kube-proxy v0.0.0 => k8s.io/kube-proxy v0.0.0-20200121202405-597cb7b43db3
	k8s.io/kube-scheduler v0.0.0 => k8s.io/kube-scheduler v0.0.0-20200121202948-05dd8b0a4787
	k8s.io/kubectl v0.0.0 => k8s.io/kubectl v0.0.0-20200121205541-a36079a4286a
	k8s.io/kubelet v0.0.0 => k8s.io/kubelet v0.0.0-20200121202654-3d0d0a3a4b44
	k8s.io/legacy-cloud-providers v0.0.0 => k8s.io/legacy-cloud-providers v0.0.0-20200121204546-147d309c2148
	k8s.io/metrics v0.0.0 => k8s.io/metrics v0.0.0-20200121201502-3a7afb0af1bc
	k8s.io/sample-apiserver v0.0.0 => k8s.io/sample-apiserver v0.0.0-20200121200150-07ea3fc70559
)
