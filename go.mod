module github.com/ISE-SMILE/corral

go 1.17

//Core requrimentes
require (
	github.com/dustin/go-humanize v1.0.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
)

//AWS Support
require (
	github.com/aws/aws-lambda-go v1.24.0
	github.com/aws/aws-sdk-go v1.38.69
)

//OpenWhisk Support
require github.com/apache/openwhisk-client-go v0.0.0-20210313152306-ea317ea2794c

//Cache Support (Redis)
require (
	github.com/go-redis/redis/v8 v8.11.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/mattetti/filebuffer v1.0.1
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

require (
	golang.org/x/mod v0.4.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.27.1
)

require (
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fatih/color v1.12.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hokaccha/go-prettyjson v0.0.0-20210113012101-fb4e108d2519 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.13 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/nicksnyder/go-i18n v1.10.1 // indirect
	github.com/pelletier/go-toml v1.9.3 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/smartystreets/assertions v1.2.0 // indirect
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023 // indirect
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22 // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/ini.v1 v1.62.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

//fixes an issue with the renaiming of gonostic, longterm we prob. need to isolate k8s deps from this as it increases binary and deps significanlty
replace github.com/googleapis/gnostic v0.5.6 => github.com/googleapis/gnostic v0.5.5

//replace github.com/mittwald/go-helm-client v0.8.0 => github.com/tawalaya/go-helm-client v0.8.1-0.20210712123422-3ceb0a361005

//replace helm.sh/helm/v3 v3.6.2 => github.com/tawalaya/helm/v3 v3.6.1-0.20210712122657-0c8e3e9a7eb4
