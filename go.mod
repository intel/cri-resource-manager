module github.com/intel/cri-resource-manager

go 1.12

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf v1.2.0 // indirect
	github.com/golang/protobuf v1.3.1
	github.com/google/go-cmp v0.3.0
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/json-iterator/go v1.1.6 // indirect

	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pkg/errors v0.8.0
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829 // indirect
	go.opencensus.io v0.22.0
	golang.org/x/net v0.0.0-20190501004415-9ce7a6920f09
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	golang.org/x/sys v0.0.0-20190502145724-3ef323f4f1fd
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	google.golang.org/appengine v1.5.0 // indirect
	google.golang.org/grpc v1.20.1
	gopkg.in/inf.v0 v0.9.1 // indirect

	k8s.io/api v0.0.0-20190626000116-40a48860b5abbba9aa891b02b32da429b08d96a0
	k8s.io/apiextensions-apiserver v0.0.0-20190626090132-ae1f9335ecc19eb6255cfe787574b8c4645dbca3 // indirect
	k8s.io/apimachinery v0.0.0-20190624085041-6a84e37a896db9780c75367af8d2ed2bb944022e
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/cloud-provider v0.0.0-20190503112208-4f570a5e56943bec25379d591bc7c7a1cbf27b3a // indirect
	k8s.io/cri-api v0.0.0-20190620080320-8a10675a4b1e
	k8s.io/kubernetes v1.14.1
)
