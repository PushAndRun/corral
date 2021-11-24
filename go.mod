module github.com/ISE-SMILE/corral

go 1.15

require (
	github.com/apache/openwhisk-client-go v0.0.0-20210313152306-ea317ea2794c
	github.com/aws/aws-lambda-go v1.24.0
	github.com/aws/aws-sdk-go v1.38.69
	github.com/containerd/containerd v1.5.8 // indirect
	github.com/docker/docker v20.10.11+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.12.0 // indirect
	github.com/go-redis/redis/v8 v8.11.0
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/mattetti/filebuffer v1.0.1
	github.com/mattn/go-isatty v0.0.13 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mittwald/go-helm-client v0.8.2
	github.com/sirupsen/logrus v1.8.1
	github.com/smartystreets/assertions v1.2.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	golang.org/x/mod v0.4.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/yaml.v2 v2.4.0
	helm.sh/helm/v3 v3.7.0
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
)

//fixes an issue with the renaiming of gonostic, longterm we prob. need to isolate k8s deps from this as it increases binary and deps significanlty
replace github.com/googleapis/gnostic v0.5.6 => github.com/googleapis/gnostic v0.5.5

//replace github.com/mittwald/go-helm-client v0.8.0 => github.com/tawalaya/go-helm-client v0.8.1-0.20210712123422-3ceb0a361005

//replace helm.sh/helm/v3 v3.6.2 => github.com/tawalaya/helm/v3 v3.6.1-0.20210712122657-0c8e3e9a7eb4
