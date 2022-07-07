package corwhisk

import (
	"context"
	"github.com/ISE-SMILE/corral/api"
	"github.com/apache/openwhisk-client-go/whisk"
	"io"
	"time"
)

type WhiskClientApi interface {
	//Invoke invokes a function of name with a payload, returing the result body as a reader or an error
	Invoke(name string, payload api.Task) (io.ReadCloser, error)

	//PollActivation uses a exponential backoff algorithm to poll activationID for MAX_RETRIES times to return the result body of an activation or an error
	PollActivation(activationID string) (io.ReadCloser, error)

	//ReceiveUntil (used for Asycn Invocation for platforms that support the Callback API) collected messages recived from the callback endpoint until when() is true or the timeout is reached
	ReceiveUntil(when func() bool, timeout *time.Duration) chan io.ReadCloser

	//InvokeAsBatch (only for platforms that support Batching) sends multiple Invocations at once, returing a list of activation IDs or an error
	InvokeAsBatch(name string, payload []api.Task) ([]interface{}, error)
	//InvokeAsync invokes a single function of name with payload asyncronously, returing the activation ID or an error
	InvokeAsync(name string, payload api.Task) (interface{}, error)

	//DeployFunction deploys a function based on the given config
	DeployFunction(conf WhiskFunctionConfig) error
	//DeleteFunction removes function with the provided name
	DeleteFunction(name string) error

	//Hint is used to trigger a hint request for function `fname` on platforms that support this feature.
	Hint(fname string, payload interface{}, hint *int) (io.ReadCloser, error)

	//Reset resets the client, closing all open connections and recievers
	Reset() error
}

type WhiskClientConfig struct {
	RequestPerMinute int64
	ConcurrencyLimit int

	Host      string
	Token     string
	Namespace string

	Context           context.Context
	RemoteLoggingHost string
	Polling           api.PollingStrategy

	BatchRequestFeature    bool
	MultiDeploymentFeature bool
	DataPreloadingFeature  bool
	HintingFeature         bool

	WriteMetrics bool

	Address *string
}

type WhiskCacheConfigInjector interface {
	api.CacheConfigInjector
	ConfigureWhisk(action *whisk.Action) error
}

type WhiskFunctionConfig struct {
	Memory              int
	Timeout             int
	FunctionName        string
	CacheConfigInjector api.CacheConfigInjector
}

type BatchRequest struct {
	Payloads []WhiskPayload `json:"payloads"`
}

type WhiskPayload struct {
	Value api.Task           `json:"value"`
	Env   map[string]*string `json:"env"`
}
