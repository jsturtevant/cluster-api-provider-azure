module sigs.k8s.io/cluster-api-provider-azure

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v42.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.10.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.2
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/golang/mock v1.4.3
	github.com/google/go-cmp v0.4.0
	github.com/google/gofuzz v1.1.0
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.3
	github.com/stretchr/testify v1.5.1 // indirect
	golang.org/x/crypto v0.0.0-20200302210943-78000ba7a073
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	k8s.io/api v0.18.2
	k8s.io/apiextensions-apiserver v0.18.2 // indirect
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	k8s.io/component-base v0.18.2
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	sigs.k8s.io/cluster-api v0.3.6
	sigs.k8s.io/controller-runtime v0.5.2
)

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.0.1+incompatible
