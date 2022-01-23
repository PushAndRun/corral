package corwhisk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/compute/build"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/apache/openwhisk-client-go/whisk"
	log "github.com/sirupsen/logrus"
)

const MaxPullRetries = 4

type WhiskClient struct {
	Client           *whisk.Client
	spawn            *ConcurrentRateLimiter
	ConcurrencyLimit *int

	ctx context.Context

	remoteLoggingHost string

	address     *string
	server      net.Listener
	activations chan io.ReadCloser

	multiDeploymentFeature bool
	deployments            map[string][]WhiskFunctionConfig

	metrics *api.Metrics
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

	var metrics *api.Metrics
	if conf.WriteMetrics {
		metrics, err = api.CollectMetrics(map[string]string{
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
		Client:                 client,
		multiDeploymentFeature: conf.MultiDeploymentFeature,
		spawn:                  limiter,
		ctx:                    conf.Context,
		address:                conf.Address,
		activations:            make(chan io.ReadCloser),
		metrics:                metrics,
		ConcurrencyLimit:       &conf.ConcurrencyLimit,
	}
}

func (l *WhiskClient) Invoke(name string, payload interface{}) (io.ReadCloser, error) {
	return l.tryInvoke(name, l.preparePayload(payload))
}

func (l *WhiskClient) InvokeAsync(name string, payload interface{}) (interface{}, error) {

	start, fname, err := l.initInvoke(name)
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

func (l *WhiskClient) initInvoke(baseName string) (int64, string, error) {
	err := l.startCallBackServer()
	if err != nil {
		return -1, "", err
	}

	err = l.spawn.Wait(l.ctx)
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

func (l *WhiskClient) InvokeAsBatch(name string, payloads []interface{}) ([]interface{}, error) {
	start, fname, err := l.initInvoke(name)

	preparedPayloads := make([]WhiskPayload, len(payloads))
	for i, payload := range payloads {
		preparedPayloads[i] = l.preparePayload(payload)
	}

	req := BatchRequest{
		preparedPayloads,
	}

	actionName := (&url.URL{Path: fname}).String()
	route := fmt.Sprintf("actions/%s?blocking=%t&result=%t&batch=%t", actionName, false, false, true)
	request, err := l.Client.NewRequest(http.MethodPost, route, req, true)

	if err != nil {
		return nil, err
	}

	var res map[string]interface{}
	response, err := l.Client.Do(request, res, false)
	if err != nil {
		return nil, err
	}

	if response.StatusCode >= 300 {
		log.Debugf("failed to batch invoce request...")
		return nil, fmt.Errorf("failed to batch requests")
	}

	activations := res["activations"].([]interface{})

	for _, activation := range activations {
		l.startMetricTrace(activation, start)
	}

	return activations, nil
}

func (l *WhiskClient) DeployFunction(conf WhiskFunctionConfig) error {
	buildPackage, codeHashDigest, err := build.BuildPackage("exec")
	if err != nil {
		return err
	}

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
			log.Debugf("Failed to fetch activation metrics for %s", id)
			return
		}

		l.collectInvocation(invoke, time.Now().UnixMilli())

	}

}

func (l *WhiskClient) tryInvoke(name string, invocation WhiskPayload) (io.ReadCloser, error) {
	failures := make([]error, 0)
	for i := 0; i < MaxRetries; i++ {
		rstart, fname, err := l.initInvoke(name)
		invoke, response, err := l.Client.Actions.Invoke(fname, invocation, true, true)
		rend := time.Now().UnixMilli()

		if response == nil && err != nil {
			failures = append(failures, err)
			log.Warnf("failed [%d/%d]", i, MaxRetries)
			log.Debugf("%+v", err)
			continue
		}

		if response != nil {
			log.Debugf("invoked %s - %d", fname, response.StatusCode)
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
					l.startMetricTrace(invoke["activationId"], rstart)
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

func (l *WhiskClient) selectDeployment(name string) string {
	//TODO: for now we do random selection ...
	configs := l.deployments[name]

	config := configs[rand.Intn(len(configs))]

	return config.FunctionName
}
