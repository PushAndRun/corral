package corwhisk

import (
	"context"
	"github.com/ISE-SMILE/corral/api"
	"github.com/apache/openwhisk-client-go/whisk"
	"io"
)

type WhiskClientApi interface {
	Invoke(name string, payload interface{}) (io.ReadCloser, error)

	InvokeAsBatch(name string, payload []interface{}) ([]interface{}, error)
	InvokeAsync(name string, payload interface{}) (interface{}, error)
	ReceiveUntil(when func() bool) chan io.ReadCloser

	DeployFunction(conf WhiskFunctionConfig) error
	DeleteFunction(name string) error
}

type WhiskClientConfig struct {
	RequestPerMinute int64
	ConcurrencyLimit int

	Host      string
	Token     string
	Namespace string

	Context           context.Context
	RemoteLoggingHost string

	BatchRequestFeature    bool
	MultiDeploymentFeature bool
	DataPreloadingFeature  bool

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
	Value interface{}        `json:"value"`
	Env   map[string]*string `json:"env"`
}
