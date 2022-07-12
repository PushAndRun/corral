module github.com/ISE-SMILE/corral

go 1.17

//Core requrimentes
require (
	github.com/dustin/go-humanize v1.0.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.12.0
	github.com/stretchr/testify v1.7.5
)

//Polling experiments
require github.com/dnlo/struct2csv v0.0.0-20190928115744-2f584471b24e

//AWS Support
require (
	github.com/aws/aws-lambda-go v1.32.0
	github.com/aws/aws-sdk-go v1.44.42
)

//OpenWhisk Support
require github.com/apache/openwhisk-client-go v0.0.0-20211007130743-38709899040b

//Cache Support (Redis)
require (
	github.com/go-redis/redis/v8 v8.11.5
	github.com/hashicorp/golang-lru v0.5.4
	github.com/mattetti/filebuffer v1.0.1
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/imdario/mergo v0.3.13
	golang.org/x/mod v0.5.1
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858
	google.golang.org/grpc v1.47.0
	google.golang.org/protobuf v1.28.0
)

require (
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hokaccha/go-prettyjson v0.0.0-20211117102719-0474bc63780f // indirect
	github.com/iancoleman/strcase v0.2.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/nicksnyder/go-i18n v1.10.1 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.4.0 // indirect
	golang.org/x/net v0.0.0-20220624214902-1bab6f366d9e // indirect
	golang.org/x/sys v0.0.0-20220624220833-87e55d714810 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220609144429-65e65417b02f // indirect
	google.golang.org/genproto v0.0.0-20220624142145-8cd45d7dbd1f // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

//fixes an issue with the renaiming of gonostic, longterm we prob. need to isolate k8s deps from this as it increases binary and deps significanlty
replace github.com/googleapis/gnostic v0.5.6 => github.com/googleapis/gnostic v0.5.5

//replace github.com/mittwald/go-helm-client v0.8.0 => github.com/tawalaya/go-helm-client v0.8.1-0.20210712123422-3ceb0a361005

//replace helm.sh/helm/v3 v3.6.2 => github.com/tawalaya/helm/v3 v3.6.1-0.20210712122657-0c8e3e9a7eb4
