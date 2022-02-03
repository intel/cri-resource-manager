module github.com/gardener/gardener-extension-cri-rm

go 1.16

require (
	github.com/gardener/gardener v1.36.0
	github.com/go-logr/logr v0.4.0
	github.com/spf13/cobra v1.2.1
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime v0.10.2
)

replace (
	github.com/gardener/gardener-resource-manager/api => github.com/gardener/gardener-resource-manager/api v0.25.0
	k8s.io/api => k8s.io/api v0.22.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.2
	k8s.io/apiserver => k8s.io/apiserver v0.22.2
	k8s.io/client-go => k8s.io/client-go v0.22.2
	k8s.io/code-generator => k8s.io/code-generator v0.22.2
	k8s.io/component-base => k8s.io/component-base v0.22.2
	k8s.io/helm => k8s.io/helm v2.13.1+incompatible
)
