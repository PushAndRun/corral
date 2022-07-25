package corwhisk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/compute/build"

	"github.com/spf13/viper"

	"github.com/apache/openwhisk-client-go/whisk"
	log "github.com/sirupsen/logrus"
)

const MaxPullRetries = 3

type WhiskClient struct {
	Client           *whisk.Client
	spawn            *ConcurrentRateLimiter
	ConcurrencyLimit *int

	ctx context.Context

	remoteLoggingHost string

	address         *string
	server          *net.TCPListener
	activations     chan io.ReadCloser
	receiverContext context.Context
	receiverCancel  context.CancelFunc

	multiDeploymentFeature bool
	hintingFeature         bool
	deployments            map[string][]WhiskFunctionConfig
	memory                 int

	polling api.PollingStrategy

	metrics *api.Metrics
	sync.Mutex
}

var propsPath string

// MaxLambdaRetries is the number of times to try invoking a function
// before giving up and returning an error
const MaxRetries = 3

// NewWhiskClient initializes a new openwhisk client
func NewWhiskClient(conf WhiskClientConfig) WhiskClientApi {
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

	receive, cnl := context.WithCancel(conf.Context)

	var metrics *api.Metrics
	if conf.WriteMetrics {
		metrics, err = api.CollectMetrics(map[string]string{
			"RId":    "request id",
			"eLat":   "execution latecncy",
			"rEnd":   "request completed",
			"rStart": "request submitted",
			"dLat":   "delivery latency",
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
		Client:                 client,
		multiDeploymentFeature: conf.MultiDeploymentFeature,
		hintingFeature:         conf.HintingFeature,
		spawn:                  limiter,
		ctx:                    conf.Context,
		receiverContext:        receive,
		receiverCancel:         cnl,
		address:                conf.Address,
		activations:            make(chan io.ReadCloser),
		metrics:                metrics,
		ConcurrencyLimit:       &conf.ConcurrencyLimit,
		remoteLoggingHost:      conf.RemoteLoggingHost,
		polling:                conf.Polling,
	}
}

//Reset closes all open connections and terminates all Receivers
func (l *WhiskClient) Reset() error {
	l.Lock()
	defer l.Unlock()

	l.receiverCancel()
	l.server.Close()

	//reset server and receiver context
	l.server = nil
	l.receiverContext, l.receiverCancel = context.WithCancel(l.ctx)

	return nil
}

func (l *WhiskClient) Hint(fname string, payload interface{}, hint *int) (io.ReadCloser, error) {
	if l.hintingFeature {
		batch := 1
		header := make(map[string]interface{})
		if hint != nil {
			batch = *hint

		}

		header["X-Corral-Hint"] = batch
		_, fname, err := l.initInvoke(fname, batch)

		body, err := l.internalInvoke(fname, payload, header, false, true)
		for i := 0; i < batch; i++ {
			l.spawn.TryAllow()
		}
		//we immediately free up a ticket, we just want to make sure we don't trigger a rate limit

		if err != nil {
			return nil, err
		}
		return body, nil
	} else {
		return nil, fmt.Errorf("hinting feature is not enabled")
	}
}

func (l *WhiskClient) Invoke(name string, payload api.Task) (io.ReadCloser, error) {
	return l.tryInvoke(name, l.preparePayload(payload))
}

func (l *WhiskClient) InvokeAsync(name string, payload api.Task) (interface{}, error) {

	start, fname, err := l.initInvoke(name, 1)
	if err != nil {
		return nil, err
	}

	inv, response, err := l.Client.Actions.Invoke(fname, l.preparePayload(payload), false, false)
	if err != nil {
		return nil, fmt.Errorf("could not invoke action - %+v", err)
	}

	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("error invoking action %s - %s", fname, response.Status)
	}

	if inv != nil {
		activationId := inv["activationId"]
		l.startMetricTrace(activationId, start)
		return activationId, nil
	} else {
		return "", nil
	}

}

func (l *WhiskClient) initInvoke(baseName string, n int) (int64, string, error) {
	err := l.startCallBackServer()
	if err != nil {
		return -1, "", err
	}

	err = l.spawn.WaitN(l.ctx, n)
	if err != nil {
		return -1, "", err
	}

	var functionName string = baseName
	//check if we run with multi deployments
	if l.multiDeploymentFeature {
		functionName = l.selectDeployment(baseName)
	}
	start := time.Now().UnixMilli()

	return start, functionName, err
}

func (l *WhiskClient) InvokeAsBatch(name string, payloads []api.Task) ([]interface{}, error) {
	start, fname, err := l.initInvoke(name, len(payloads))
	if err != nil {
		log.Debugf("failed to init invoke %s", err)
		return nil, err
	}

	preparedPayloads := make([]WhiskPayload, len(payloads))
	for i, payload := range payloads {
		preparedPayloads[i] = l.preparePayload(payload)
	}

	req := BatchRequest{
		preparedPayloads,
	}

	body, err := l.internalInvoke(fname, req, nil, true, false)
	if err != nil {
		return nil, err
	}
	log.Debugf("invoked batch at %s", name)

	var res map[string]interface{}
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return []interface{}{}, nil
	}
	json.Unmarshal(data, &res)

	activations := res["activations"].([]interface{})
	log.Debugf("got %d activations for %d invocations", len(activations), len(payloads))
	invocations := make([]interface{}, 0)
	for _, activation := range activations {
		if invoke, ok := activation.(map[string]interface{}); ok {
			if id, ok := invoke["activationId"]; ok {
				l.startMetricTrace(id, start)
				invocations = append(invocations, id)
			}
		}

	}

	return invocations, nil
}

func (l *WhiskClient) internalInvoke(fname string, payload interface{}, header map[string]interface{}, batch, hint bool) (io.ReadCloser, error) {
	if batch && hint {
		return nil, fmt.Errorf("batch and hint are mutually exclusive")
	}

	actionName := (&url.URL{Path: fname}).String()
	route := fmt.Sprintf("actions/%s?blocking=%t&result=%t&batch=%t&hint=%t", actionName, false, false, batch, hint)
	request, err := l.Client.NewRequest(http.MethodPost, route, payload, true)

	if header != nil {
		for k, v := range header {
			request.Header.Set(k, fmt.Sprintf("%+v", v))
		}
	}

	if err != nil {
		return nil, err
	}
	//TODO: we might have to set v here...
	response, err := l.Client.Do(request, nil, false)
	if err != nil {
		if response != nil {
			data, _ := ioutil.ReadAll(response.Body)
			return nil, fmt.Errorf("%+v - %s", err, string(data))
		}
		return nil, err
	}

	if response.StatusCode >= 300 {
		data, _ := ioutil.ReadAll(response.Body)
		return nil, fmt.Errorf("%s - %s", response.Status, string(data))
	}

	return response.Body, nil
}

func (l *WhiskClient) DeployFunction(conf WhiskFunctionConfig) error {
	buildPackage, codeHashDigest, err := build.BuildPackage("exec")
	if err != nil {
		log.Debugf("failed to build package %+v", err)
		return err
	}

	l.memory = conf.Memory

	if !l.multiDeploymentFeature {
		return l.deployFunction(conf, buildPackage, codeHashDigest)
	} else {
		if l.deployments == nil {
			l.deployments = make(map[string][]WhiskFunctionConfig)

			n := viper.GetInt("numMultiDeployments")
			m := (conf.Memory - viper.GetInt("maxMemory")) / n
			confs := make([]WhiskFunctionConfig, n)
			confs[0] = conf

			for i := 1; i < n; i++ {
				conf.Memory += m
				confs[i] = conf
				confs[i].FunctionName = fmt.Sprintf("%s_%d", conf.FunctionName, 0)
			}

			confs[0].FunctionName = fmt.Sprintf("%s_%d", conf.FunctionName, 0)

			l.deployments[conf.FunctionName] = confs
		}
		for _, config := range l.deployments[conf.FunctionName] {
			err := l.deployFunction(config, buildPackage, codeHashDigest)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func (l *WhiskClient) deployFunction(conf WhiskFunctionConfig, buildPackage []byte, codeHashDigest string) error {
	log.Debugf("deploying function %s - %+v", conf.FunctionName, conf)
	actionName, namespace := GetQualifiedName(conf.FunctionName)

	if conf.Memory == 0 {
		conf.Memory = 192
	}

	if conf.Timeout == 0 {
		conf.Timeout = int(time.Second * 30)
	}

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
		Timeout: &conf.Timeout,
		Memory:  &conf.Memory,
		Logsize: nil,
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

	if l.address != nil {
		addressAnnotation := whisk.KeyValue{
			Key:   "address",
			Value: *l.address,
		}
		action.Annotations = action.Annotations.AddOrReplace(&addressAnnotation)
	}

	if conf.CacheConfigInjector != nil {
		log.Debug("injecting cache config")
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
	if l.multiDeploymentFeature {
		errors := make([]error, 0)
		for _, config := range l.deployments[name] {
			_, err := l.Client.Actions.Delete(config.FunctionName)
			if err != nil {
				errors = append(errors, err)
			}
		}

		if len(errors) > 0 {
			return fmt.Errorf("%+v", errors)
		}
		return nil
	} else {
		_, err := l.Client.Actions.Delete(name)
		return err
	}
}

func (l *WhiskClient) startMetricTrace(activationId interface{}, start int64) {
	if l.metrics != nil {
		l.metrics.Collect(map[string]interface{}{
			"RId":    activationId,
			"rStart": start,
		})
	}
}

func (l *WhiskClient) fetchActivationMetrics(id string) {
	if l.metrics != nil {
		invoke, _, err := l.Client.Activations.Get(id)
		if err != nil {
			return
		}

		l.collectInvocation(invoke, time.Now().UnixMilli(), 0)

	}

}

func (l *WhiskClient) tryInvoke(name string, invocation WhiskPayload) (io.ReadCloser, error) {
	failures := make([]error, 0)
	for i := 0; i < MaxRetries; i++ {
		rstart, fname, err := l.initInvoke(name, 1)
		//change to false to switch from synchronous calls to polling only
		invoke, response, err := l.Client.Actions.Invoke(fname, invocation, false, false)
		rend := time.Now().UnixMilli()
		log.Debugf("invoke %s took %d ms", fname, rend-rstart)
		if response == nil && err != nil {
			failures = append(failures, err)
			log.Warnf("failed [%d/%d]", i, MaxRetries)
			log.Debugf("%+v", err)
			continue
		}

		if response != nil {
			log.Debugf("%d - %+v", response.StatusCode, invoke)
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
				l.spawn.TryAllow()
				return response.Body, nil
			} else if response.StatusCode == 202 {
				if id, ok := invoke["activationId"]; ok {
					l.startMetricTrace(invoke["activationId"], rstart)
					l.polling.TaskUpdate(api.TaskInfo{
						RId:    id.(string),
						JobId:  invocation.Value.JobId,
						TaskId: invocation.Value.TaskId,
						Phase:  int(invocation.Value.Phase),
					})
					activation, err := l.PollActivation(id.(string))
					if err != nil {
						failures = append(failures, err)
						l.spawn.TryAllow()
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
			l.polling.TaskUpdate(api.TaskInfo{
				JobId:  invocation.Value.JobId,
				TaskId: invocation.Value.TaskId,
				Phase:  int(invocation.Value.Phase),
				Failed: true,
			})
			//give back an token, since we failed, (note this could lead to to much call pressure, if backoff is to short and failiures happend to often)
			l.spawn.Allow()
		}
	}

	msg := &strings.Builder{}
	for _, err := range failures {
		msg.WriteString(err.Error())
		msg.WriteRune('\t')
	}
	return nil, fmt.Errorf(msg.String())

}

func (l *WhiskClient) PollActivation(activationID string) (io.ReadCloser, error) {
	log.Debugf("polling Activation %s", activationID)
	for x := 0; x < MaxPullRetries; x++ {

		invoke, response, err := l.Client.Activations.Get(activationID)

		if err != nil || response.StatusCode == 404 {
			if err != nil {
				log.Debugf("failed to poll %+v", err)
			}
			poll, err := l.polling.Poll(l.ctx, activationID)
			<-poll
			if err != nil {
				log.Debugf("failed to wait for poll %+v", err)
			}

		} else if response.StatusCode == 200 {

			log.Debugf("polled %s successfully", activationID)
			l.spawn.TryAllow()
			l.polling.SetFinalPollTime(activationID, time.Now().UnixNano())
			l.collectInvocation(invoke, time.Now().UnixMilli(), x)
			marshal, err := json.Marshal(invoke.Result)
			if err == nil {
				return ioutil.NopCloser(bytes.NewReader(marshal)), nil
			} else {
				return nil, fmt.Errorf("failed to fetch activation %s due to %f", activationID, err)
			}
		}
	}
	return nil, fmt.Errorf("could not fetch activation after %d ties", MaxPullRetries)
}

func (l *WhiskClient) collectInvocation(invoke *whisk.Activation, rend int64, prematurePolls int) {
	if l.metrics != nil {
		l.metrics.Collect(map[string]interface{}{
			"RId":    invoke.ActivationID,
			"eStart": invoke.Start,
			"eEnd":   invoke.End,
			"eLat":   invoke.End - invoke.Start,
			"rEnd":   rend,
			"dLat":   rend - invoke.End,
			"pPolls": prematurePolls,
			"memory": l.memory,
		})
	}
}

func (l *WhiskClient) startCallBackServer() error {
	l.Lock()
	defer l.Unlock()
	if l.server != nil {
		return nil
	}

	if l.address != nil {

		//var lc net.ListenConfig
		//srv, err := lc.Listen(l.receiverContext, "tcp", *l.address)
		addr, err := net.ResolveTCPAddr("tcp", *l.address)
		srv, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return err
		}
		l.server = srv
		go l.accept()
		return nil
	}

	return nil
}

//ReceiveUntil will copy all io.ReadCloser received until the when function is true  a timeout is reached.
//In these cases the channal will be closed to singal the end to the consumer.
//if the timeout is not set this function will timeout after 15 minutes.
func (l *WhiskClient) ReceiveUntil(when func() bool, timeout *time.Duration) chan io.ReadCloser {
	buffer := make(chan io.ReadCloser)
	var t time.Duration
	if timeout != nil {
		t = *timeout
	} else {
		t = time.Minute * 15 //max openwhisk duration
	}

	go func(timeout time.Duration) {
		defer close(buffer)
		for {
			select {
			case b, ok := <-l.activations:
				if !ok {
					return
				}

				buffer <- b
				if when != nil && when() {
					return
				}
			case <-time.After(timeout):
				return
			case <-l.receiverContext.Done():
				return
			}
		}
	}(t)

	return buffer
}

func (l *WhiskClient) accept() {
	if l.server == nil {
		return
	}
	for {
		conn, err := l.server.AcceptTCP()
		if err != nil {
			select {
			case <-l.ctx.Done():
				return
			case <-l.receiverContext.Done():
				return
			default:
				log.Debugf("server failed to accept %+v", err)
			}
		} else {
			go l.handleConnection(conn)
			log.Debugf("new connection %+v", conn.RemoteAddr())
		}
	}
}

func (l *WhiskClient) handleConnection(conn *net.TCPConn) {
	if conn == nil {
		return
	}
	defer conn.Close()

	_ = conn.SetReadBuffer(1024 * 64)
	_ = conn.SetNoDelay(true)
	_ = conn.SetKeepAlive(true)
	_ = conn.SetReadDeadline(time.Now().Add(viper.GetDuration("acceptTimeout")))

	scanner := bufio.NewScanner(conn)

	for {
		select {
		case <-l.ctx.Done():
			log.Debugf("client context closed, closing connection %+v", conn.RemoteAddr())
			return
		case <-l.receiverContext.Done():
			log.Debugf("receiver context closed, closing connection %+v", conn.RemoteAddr())
			return
		default:
			if scanner.Scan() {
				buf := new(bytes.Buffer)
				buf.Write(scanner.Bytes())
				_ = conn.SetReadDeadline(time.Now().Add(viper.GetDuration("acceptTimeout")))
				l.activations <- io.NopCloser(buf)
				l.spawn.TryAllow()
			} else {
				if scanner.Err() != nil {
					log.Debugf("failed to read from connection - %+v", scanner.Err())
				} else {
					log.Debugf("Connectionhandler closed EOF")
				}
				return
			}
		}
	}
}

func (l *WhiskClient) selectDeployment(name string) string {
	//TODO: for now we do random selection ...
	configs := l.deployments[name]

	config := configs[rand.Intn(len(configs))]

	return config.FunctionName
}
