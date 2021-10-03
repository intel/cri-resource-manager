module github.com/intel/cri-resource-manager

go 1.14

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.1-0.20191218042359-6151c48ac7fa
	github.com/apache/thrift v0.13.0 // indirect
	github.com/cilium/ebpf v0.2.0
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/hashicorp/go-multierror v1.1.1
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0
	github.com/intel/goresctrl v0.2.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.30.0
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	go.opencensus.io v0.22.4
	go.uber.org/zap v1.13.0 // indirect
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	golang.org/x/sys v0.0.0-20210903071746-97244b99971b
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	google.golang.org/grpc v1.33.2
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.21.0
	k8s.io/cri-api v0.21.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/kubernetes v1.21.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0 => ./pkg/topology
	google.golang.org/grpc => google.golang.org/grpc v1.26.0

	k8s.io/api v0.0.0 => k8s.io/api v0.0.0-20210408192244-5d0d5b56b511
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.0.0-20210408194826-713b97842000
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.0.0-20210329111815-e337f44144a6
	k8s.io/apiserver v0.0.0 => k8s.io/apiserver v0.0.0-20210408193723-976e7a099887
	k8s.io/cli-runtime v0.0.0 => k8s.io/cli-runtime v0.0.0-20210408195238-aa204177202e
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.0.0-20210408192749-b09a9ce3bf3b
	k8s.io/cloud-provider v0.0.0 => k8s.io/cloud-provider v0.0.0-20210408200433-a198976dd511
	k8s.io/cluster-bootstrap v0.0.0 => k8s.io/cluster-bootstrap v0.0.0-20210408200842-09dba4674599
	k8s.io/code-generator v0.0.0 => k8s.io/code-generator v0.0.0-20210329111516-cb1b268548af
	k8s.io/component-base v0.0.0 => k8s.io/component-base v0.0.0-20210408192953-c3b9e07a8fd9
	k8s.io/component-helpers v0.0.0 => k8s.io/component-helpers v0.0.0-20210408193125-05b6b618a2fa
	k8s.io/controller-manager v0.0.0 => k8s.io/controller-manager v0.0.0-20210408200210-f0e191876d59
	k8s.io/cri-api v0.0.0 => k8s.io/cri-api v0.0.0-20210329121440-254f38568068
	k8s.io/csi-translation-lib v0.0.0 => k8s.io/csi-translation-lib v0.0.0-20210408201034-1358d50096b3
	k8s.io/kube-aggregator v0.0.0 => k8s.io/kube-aggregator v0.0.0-20210408194009-110934c68703
	k8s.io/kube-controller-manager v0.0.0 => k8s.io/kube-controller-manager v0.0.0-20210408200643-d72e7d987231
	k8s.io/kube-proxy v0.0.0 => k8s.io/kube-proxy v0.0.0-20210408195631-3eab5bd43241
	k8s.io/kube-scheduler v0.0.0 => k8s.io/kube-scheduler v0.0.0-20210408200021-2d0b2fbd78d9
	k8s.io/kubectl v0.0.0 => k8s.io/kubectl v0.0.0-20210408201839-15040bc0407c
	k8s.io/kubelet v0.0.0 => k8s.io/kubelet v0.0.0-20210408195821-2f96697a6b86
	k8s.io/legacy-cloud-providers v0.0.0 => k8s.io/legacy-cloud-providers v0.0.0-20210408201324-249487b505d9
	k8s.io/metrics v0.0.0 => k8s.io/metrics v0.0.0-20210408195036-d98381db7b34
	k8s.io/mount-utils v0.0.0 => k8s.io/mount-utils v0.0.0-20210329121850-a4f0d12ea86f
	k8s.io/sample-apiserver v0.0.0 => k8s.io/sample-apiserver v0.0.0-20210408194250-ba82cf04793e
)
