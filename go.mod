module github.com/intel/cri-resource-manager

go 1.14

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.1-0.20191218042359-6151c48ac7fa
	github.com/cilium/ebpf v0.0.0-20200702112145-1c8d4c9ef775
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.4.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0
	github.com/intel/goresctrl v0.0.0-20201221180043-c1bbf3a22bce
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.8.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.14.0
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	go.opencensus.io v0.22.2
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20201015000850-e3ed0017c211
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	google.golang.org/grpc v1.27.0
	k8s.io/api v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.19.4
	k8s.io/cri-api v0.0.0
	k8s.io/klog/v2 v2.4.0
	k8s.io/kubernetes v1.19.4
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/intel/cri-resource-manager/pkg/topology v0.0.0 => ./pkg/topology
	google.golang.org/grpc => google.golang.org/grpc v1.26.0

	k8s.io/api v0.0.0 => k8s.io/api v0.0.0-20201113170447-7ec4e34ebfa0
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.0.0-20201113174919-8ce2dfad2388
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.0.0-20200821171749-b63a0c883fbf
	k8s.io/apiserver v0.0.0 => k8s.io/apiserver v0.0.0-20201113172959-d4704a3a5cde
	k8s.io/cli-runtime v0.0.0 => k8s.io/cli-runtime v0.0.0-20201113175539-7ac36a2d8758
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.0.0-20201113171635-2bb8681d6833
	k8s.io/cloud-provider v0.0.0 => k8s.io/cloud-provider v0.0.0-20201113181133-070bf588610e
	k8s.io/cluster-bootstrap v0.0.0 => k8s.io/cluster-bootstrap v0.0.0-20201113181704-acbca43bf834
	k8s.io/code-generator v0.0.0 => k8s.io/code-generator v0.0.0-20200813171329-1c3794b15e35
	k8s.io/component-base v0.0.0 => k8s.io/component-base v0.0.0-20201113171950-93d3585cb60c
	k8s.io/cri-api v0.0.0 => k8s.io/cri-api v0.0.0-20200727095024-0fd2b9b35a65
	k8s.io/csi-translation-lib v0.0.0 => k8s.io/csi-translation-lib v0.0.0-20201113182001-d03728bbcf5a
	k8s.io/kube-aggregator v0.0.0 => k8s.io/kube-aggregator v0.0.0-20201113173433-ce05bf9f9509
	k8s.io/kube-controller-manager v0.0.0 => k8s.io/kube-controller-manager v0.0.0-20201113181424-f91ce16d8de8
	k8s.io/kube-proxy v0.0.0 => k8s.io/kube-proxy v0.0.0-20201113180133-d8e6aa5a8916
	k8s.io/kube-scheduler v0.0.0 => k8s.io/kube-scheduler v0.0.0-20201113180716-8fee79fb9051
	k8s.io/kubectl v0.0.0 => k8s.io/kubectl v0.0.0-20201113183129-6c3805304969
	k8s.io/kubelet v0.0.0 => k8s.io/kubelet v0.0.0-20201113180421-609649667e1d
	k8s.io/legacy-cloud-providers v0.0.0 => k8s.io/legacy-cloud-providers v0.0.0-20201113182353-b77454164b5d
	k8s.io/metrics v0.0.0 => k8s.io/metrics v0.0.0-20201113175240-4a8e322202a4
	k8s.io/sample-apiserver v0.0.0 => k8s.io/sample-apiserver v0.0.0-20201113174004-1f754d3b7d52
)
