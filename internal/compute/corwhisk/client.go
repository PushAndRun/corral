package corwhisk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/internal/compute/build"
	"github.com/ISE-SMILE/corral/internal/cormetics"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/apache/openwhisk-client-go/whisk"
	log "github.com/sirupsen/logrus"
)

const MaxPullRetries = 4

type WhiskClientApi interface {
	Invoke(name string, payload interface{}) (io.ReadCloser, error)
	DeployFunction(conf WhiskFunctionConfig) error
	DeleteFunction(name string) error
}

type WhiskClient struct {
	Client           *whisk.Client
	spawn            *ConcurrentRateLimiter
	ConcurrencyLimit *int

	ctx context.Context

	remoteLoggingHost string

	address     *string
	server      net.Listener
	activations chan io.ReadCloser

	metrics *cormetics.Metrics
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
	MultiDeploymentFeatrue bool
	WriteMetics            bool

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

var propsPath string

func init() {
	home, err := os.UserHomeDir()
	if err == nil {
		propsPath = filepath.Join(home, ".wskprops")
	} else {
		//best effort this will prop. work on unix and osx ;)
		propsPath = filepath.Join("~", ".wskprops")
	}
}

func readProps(in io.ReadCloser) map[string]string {
	defer in.Close()

	props := make(map[string]string)

	reader := bufio.NewScanner(in)

	for reader.Scan() {
		line := reader.Text()
		data := strings.SplitN(line, "=", 2)
		if len(data) < 2 {
			//This might leek user private data into a log...
			log.Errorf("could not read prop line %s", line)
		}
		props[data[0]] = data[1]
	}
	return props
}

func readEnviron() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		data := strings.SplitN(e, "=", 2)
		if len(data) < 2 {
			//This might leek user private data into a log...
			log.Errorf("could not read prop line %s", e)
		}
		env[data[0]] = data[1]
	}
	return env
}

func whiskClient(conf WhiskClientConfig) (*whisk.Client, error) {
	// lets first check the config
	host := conf.Host
	token := conf.Token
	var namespace = "_"

	if token == "" {
		//2. check if wskprops exsist
		if _, err := os.Stat(propsPath); err == nil {
			//. attempt to read and parse props
			if props, err := os.Open(propsPath); err == nil {
				host, token, namespace = setAtuhFromProps(readProps(props))
			}
			//3. fallback try to check the env for token
		} else {
			host, token, namespace = setAtuhFromProps(readEnviron())
		}
	}

	if token == "" {
		log.Warn("did not find a token for the whisk client!")
	}

	if conf.Namespace != "" {
		namespace = conf.Namespace
	}

	baseurl, _ := whisk.GetURLBase(host, "/api")
	clientConfig := &whisk.Config{
		Namespace:        namespace,
		AuthToken:        token,
		Host:             host,
		BaseURL:          baseurl,
		Version:          "v1",
		Verbose:          true,
		Insecure:         true,
		UserAgent:        "Golang/Smile cli",
		ApigwAccessToken: "Dummy Token",
	}

	client, err := whisk.NewClient(http.DefaultClient, clientConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

//check props and env vars for relevant infomation ;)
func setAtuhFromProps(auth map[string]string) (string, string, string) {
	var host string
	var token string
	var namespace string
	if apihost, ok := auth["APIHOST"]; ok {
		host = apihost
	} else if apihost, ok := auth["__OW_API_HOST"]; ok {
		host = apihost
	}
	if apitoken, ok := auth["AUTH"]; ok {
		token = apitoken
	} else if apitoken, ok := auth["__OW_API_KEY"]; ok {
		token = apitoken
	}
	if apinamespace, ok := auth["NAMESPACE"]; ok {
		namespace = apinamespace
	} else if apinamespace, ok := auth["__OW_NAMESPACE"]; ok {
		namespace = apinamespace
	}
	return host, token, namespace
}

// MaxLambdaRetries is the number of times to try invoking a function
// before giving up and returning an error
const MaxRetries = 3

// NewWhiskClient initializes a new openwhisk client
func NewWhiskClient(conf WhiskClientConfig) *WhiskClient {
	client, err := whiskClient(conf)
	if err != nil {
		panic(fmt.Errorf("could not init whisk client - %+v", err))
	}
	client.Verbose = true
	client.Debug = true

	requestPerMinute := conf.RequestPerMinute
	if requestPerMinute == 0 {
		requestPerMinute = 200
	}

	ConcurrencyLimit := conf.ConcurrencyLimit
	if ConcurrencyLimit <= 0 {
		ConcurrencyLimit = 30
	}

	if conf.Context != nil {
		conf.Context = context.Background()
	}

	var metrics *cormetics.Metrics
	if conf.WriteMetics {
		metrics, err = cormetics.CollectMetrics(map[string]string{
			"RId":   "request id",
			"start": "request submitted",
			"end":   "request completed",
		})
		if err != nil {
			log.Error("could not init metrics", err)
			metrics = nil
		}
	}

	limiter, err := NewConcurrentRateLimiter(requestPerMinute, ConcurrencyLimit)
	if err != nil {
		panic(fmt.Errorf("could not init whisk client - %+v", err))
	}
	return &WhiskClient{
		Client:           client,
		spawn:            limiter,
		ctx:              conf.Context,
		address:          conf.Address,
		activations:      make(chan io.ReadCloser),
		metrics:          metrics,
		ConcurrencyLimit: &conf.ConcurrencyLimit,
	}
}

type WhiskPayload struct {
	Value interface{}        `json:"value"`
	Env   map[string]*string `json:"env"`
}

func (l *WhiskClient) Invoke(name string, payload interface{}) (io.ReadCloser, error) {
	return l.tryInvoke(name, l.preparePayload(payload))
}

func (l *WhiskClient) preparePayload(payload interface{}) WhiskPayload {
	invocation := WhiskPayload{
		Value: payload,
		Env:   make(map[string]*string),
	}

	build.InjectConfiguration(invocation.Env)
	strBool := "true"
	invocation.Env["DEBUG"] = &strBool
	if l.remoteLoggingHost != "" {
		invocation.Env["RemoteLoggingHost"] = &l.remoteLoggingHost
	}
	return invocation
}

func (l *WhiskClient) InvokeAsyn(name string, payload interface{}) error {

	err := l.startCallBackServer()
	if err != nil {
		return err
	}

	err = l.spawn.Wait(l.ctx)
	if err != nil {
		return err
	}

	start := time.Now().UnixMilli()
	inv, response, err := l.Client.Actions.Invoke(name, l.preparePayload(payload), false, false)
	if err != nil {
		return fmt.Errorf("could not invoke action - %+v", err)
	}

	if response.StatusCode >= 300 {
		return fmt.Errorf("error invoking action %s - %s", name, response.Status)
	}

	if l.metrics != nil {
		l.metrics.Collect(map[string]interface{}{
			"RId":   inv["activationId"],
			"start": start,
		})
	}

	return nil
}

// returns name and namespace based on function name
func getQualifiedName(functioname string) (string, string) {
	var namespace string
	var action string

	if strings.HasPrefix(functioname, "/") {
		parts := strings.Split(functioname, "/")
		namespace = parts[0]
		action = parts[1]
	} else {
		//no namespace set in string using _
		namespace = "_"
		action = functioname
	}

	return action, namespace

}

func (l *WhiskClient) DeployFunction(conf WhiskFunctionConfig) error {

	actionName, namespace := getQualifiedName(conf.FunctionName)

	if conf.Memory == 0 {
		conf.Memory = 192
	}

	if conf.Timeout == 0 {
		conf.Timeout = int(time.Second * 30)
	}

	buildPackage, codeHashDigest, err := build.BuildPackage("exec")

	payload := base64.StdEncoding.EncodeToString(buildPackage)

	act, _, err := l.Client.Actions.Get(conf.FunctionName, false)
	if err == nil {
		if remoteHash := act.Annotations.GetValue("codeHash"); remoteHash != nil {
			if strings.Compare(codeHashDigest, remoteHash.(string)) == 0 {
				log.Info("code hash equal, skipping deployment")
				return nil
			} else {
				log.Debugf("code hash differ %s %s, updating", codeHashDigest, remoteHash)
			}
		}
	}

	action := new(whisk.Action)

	action.Name = actionName
	action.Namespace = namespace

	action.Limits = &whisk.Limits{
		Timeout:     &conf.Timeout,
		Memory:      &conf.Memory,
		Logsize:     nil,
		Concurrency: l.ConcurrencyLimit,
	}

	var binary = true
	action.Exec = &whisk.Exec{
		Kind:       "go:1.15",
		Code:       &payload,
		Main:       "main",
		Components: nil,
		Binary:     &binary,
	}

	//allows us to check if the deployment needs to be updated
	hashAnnotation := whisk.KeyValue{
		Key:   "codeHash",
		Value: codeHashDigest,
	}

	action.Annotations = action.Annotations.AddOrReplace(&hashAnnotation)

	addressAnnotation := whisk.KeyValue{
		Key:   "address",
		Value: viper.GetString("address"),
	}
	action.Annotations = action.Annotations.AddOrReplace(&addressAnnotation)

	if conf.CacheConfigInjector != nil {
		if wi, ok := conf.CacheConfigInjector.(WhiskCacheConfigInjector); ok {
			err := wi.ConfigureWhisk(action)
			if err != nil {
				log.Warnf("failed to inject cache config into function")
				return err
			}
		} else {
			log.Errorf("cannot configure cache for this type of function, check the docs.")
			return fmt.Errorf("can't deploy function without injecting cache config")
		}
	}

	action, _, err = l.Client.Actions.Insert(action, true)

	if err != nil {
		log.Debugf("failed to deploy %s cause %+v", conf.FunctionName, err)
		return err
	}

	log.Infof("deployed %s using [%s]", conf.FunctionName, action.Name)
	return nil
}

func (l *WhiskClient) DeleteFunction(name string) error {
	_, err := l.Client.Actions.Delete(name)
	return err
}

func (l *WhiskClient) fetchActivationMetrics(id string) {
	if l.metrics != nil {
		invoke, _, err := l.Client.Activations.Get(id)
		if err != nil {
			log.Debugf("Failed to fetch activation metrics for %s", id)
			return
		}

		l.collectInvocation(invoke, time.Now().UnixMilli())

	}

}

func (l *WhiskClient) tryInvoke(name string, invocation WhiskPayload) (io.ReadCloser, error) {
	failures := make([]error, 0)
	for i := 0; i < MaxRetries; i++ {
		err := l.spawn.Wait(l.ctx)
		if err != nil {
			//wait canceld form the outside
			return nil, err
		}
		rstart := time.Now().UnixMilli()
		invoke, response, err := l.Client.Actions.Invoke(name, invocation, true, true)
		rend := time.Now().UnixMilli()

		if response == nil && err != nil {
			failures = append(failures, err)
			log.Warnf("failed [%d/%d]", i, MaxRetries)
			log.Debugf("%+v", err)
			continue
		}

		if response != nil {
			log.Debugf("invoked %s - %d", name, response.StatusCode)
			log.Debugf("%+v", invoke)
			if response.StatusCode == 200 {
				if l.metrics != nil {
					rid := response.Header.Get("X-Openwhisk-Activation-ID")

					l.metrics.Collect(map[string]interface{}{
						"RId":    rid,
						"rStart": rstart,
						"rEnd":   rend,
					})
					if rid != "" {
						go l.fetchActivationMetrics(rid)
					}
				}
				l.spawn.Allow()
				return response.Body, nil
			} else if response.StatusCode == 202 {
				if id, ok := invoke["activationId"]; ok {
					if l.metrics != nil {
						l.metrics.Collect(map[string]interface{}{
							"RId":    invoke["activationId"],
							"rStart": rstart,
						})
					}
					activation, err := l.pollActivation(id.(string))
					l.spawn.Allow()
					if err != nil {
						failures = append(failures, err)
					} else {
						return activation, nil
					}
				}
			} else {
				failures = append(failures, fmt.Errorf("failed to invoke %d %+v", response.StatusCode, response.Body))
				log.Debugf("failed [%d/%d ] times to invoke %s with %+v  %+v %+v", i, MaxRetries,
					name, invocation.Value, invoke, response)
			}
		} else {
			log.Warnf("failed [%d/%d]", i, MaxRetries)
		}
	}

	msg := &strings.Builder{}
	for _, err := range failures {
		msg.WriteString(err.Error())
		msg.WriteRune('\t')
	}
	return nil, fmt.Errorf(msg.String())

}

func (l *WhiskClient) pollActivation(activationID string) (io.ReadCloser, error) {
	//might want to configuer the backof rate?
	backoff := 4

	wait := func(backoff int) int {
		//results not here yet... keep wating
		<-time.After(time.Second * time.Duration(backoff))
		//exponential backoff of 4,16,64,256,1024 seconds
		backoff = backoff * 4
		log.Debugf("results not ready waiting for %d", backoff)
		return backoff
	}

	log.Debugf("polling Activation %s", activationID)
	for x := 0; x < MaxPullRetries; x++ {
		//err := l.spawn.Wait(l.ctx)
		//if err != nil {
		//	return nil, err
		//}
		invoke, response, err := l.Client.Activations.Get(activationID)
		//l.spawn.Allow()
		if err != nil || response.StatusCode == 404 {
			backoff = wait(backoff)
			if err != nil {
				log.Debugf("failed to poll %+v", err)
			}
		} else if response.StatusCode == 200 {
			log.Debugf("polled %s successfully", activationID)
			l.collectInvocation(invoke, time.Now().UnixMilli())
			marshal, err := json.Marshal(invoke.Result)
			if err == nil {
				return ioutil.NopCloser(bytes.NewReader(marshal)), nil
			} else {
				return nil, fmt.Errorf("failed to fetch activation %s due to %f", activationID, err)
			}
		}
	}
	return nil, fmt.Errorf("could not fetch activation after %d ties in %d", MaxPullRetries, backoff+backoff-1)
}

func (l *WhiskClient) collectInvocation(invoke *whisk.Activation, rend int64) {
	if l.metrics != nil {
		l.metrics.Collect(map[string]interface{}{
			"RId":    invoke.ActivationID,
			"eStart": invoke.Start,
			"eEnd":   invoke.End,
			"eLat":   invoke.End - invoke.Start,
			"rEnd":   rend,
			"dLat":   rend - invoke.End,
		})
	}
}

func (l *WhiskClient) startCallBackServer() error {
	if l.server != nil {
		return nil
	}

	if l.address != nil {

		srv, err := net.Listen("tcp", *l.address)
		if err != nil {
			return err
		}
		l.server = srv
		go l.accept()
		return nil
	}

	return nil
}

//ReciveUnitl will copy all io.ReadCloser received until the when function is true or until the client context is closed. In these cases the channal will be closed to singal the end to the consumer.
func (l *WhiskClient) ReceiveUntil(when func() bool) chan io.ReadCloser {
	buffer := make(chan io.ReadCloser)
	go func() {
		defer close(buffer)
		for {
			select {
			case <-l.ctx.Done():
				return //canceled
			case b := <-l.activations:
				buffer <- b
				if when != nil && when() {
					return
				}
			}
		}

	}()

	return buffer
}

func (l *WhiskClient) Close() error {
	if l.server != nil {
		err := l.server.Close()
		l.server = nil
		return err
	}

	return nil
}

func (l *WhiskClient) accept() {
	for {
		select {
		case <-l.ctx.Done():
			return
		default:
			if l.server == nil {
				log.Debugf("accept failed server is closed")
				return
			}

			conn, err := l.server.Accept()
			if err != nil {
				log.Debugf("accept failed %+v", err)
				if err == net.ErrClosed {
					return
				}
			}

			go func(conn net.Conn, activations chan io.ReadCloser) {
				if conn == nil {
					return
				}
				data, err := ioutil.ReadAll(conn)
				conn.Close()
				l.spawn.Allow()

				if err != nil {
					log.Debug("failed to read from connection")
				}
				log.Debug("got activation callback")
				activations <- io.NopCloser(bytes.NewBuffer(data))

			}(conn, l.activations)
		}
	}
}
