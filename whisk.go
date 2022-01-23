package corral

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	corwhisk2 "github.com/ISE-SMILE/corral/compute/corwhisk"
	io2 "github.com/ISE-SMILE/corral/internal/corfs"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	whiskDriver *Driver
)

func runningInWhisk() bool {
	expectedEnvVars := []string{"__OW_EXECUTION_ENV", "__OW_API_HOST"}
	for _, envVar := range expectedEnvVars {
		if os.Getenv(envVar) == "" {
			return false
		}
	}
	return true
}

var whiskNodeName string

func whiskHostID() string {
	if whiskNodeName != "" {
		return whiskNodeName
	}
	//there are two possible scenarios: docker direct or kubernetes...

	//is only present in some upstream openwhisk builds ;)
	if _, err := os.Stat("/vmhost"); err == nil {
		data, err := ioutil.ReadFile("/vmhost")
		if err == nil {
			whiskNodeName = string(bytes.Replace(data, []byte("\n"), []byte(""), -1))
			return whiskNodeName
		}
	}
	if name := os.Getenv("NODENAME"); name != "" {
		whiskNodeName = name
		return whiskNodeName
	}

	uptime := readUptime()
	if uptime != "" {
		whiskNodeName = uptime
		return whiskNodeName
	}

	whiskNodeName = "unknown"
	return whiskNodeName
}

func whiskRequestID() string {
	return os.Getenv("ActivationId")
}

func handleWhsikRequest(task task) (taskResult, error) {
	return handle(whiskDriver, whiskHostID, whiskRequestID)(task)
}

func handleWhiskHook(out io.Writer, hook func(*Job) string) {
	if whiskDriver != nil {
		job := whiskDriver.CurrentJob()
		if job != nil {
			msg := hook(job)
			if msg != "" {
				fmt.Fprintln(out, msg)
			}
		}
	}
}

func handleWhiskPause(out io.Writer) {
	handleWhiskHook(out, func(job *Job) string {
		return job.PauseFunc()
	})
}

func handleWhiskStop(out io.Writer) {
	handleWhiskHook(out, func(job *Job) string {
		return job.StopFunc()
	})
}

func handleWhiskFreshen(out io.Writer) {}

func handleWhiskHint(out io.Writer) {
	handleWhiskHook(out, func(job *Job) string {
		return job.HintFunc()
	})
}

type whiskExecutor struct {
	corwhisk2.WhiskClientApi
	functionName string
}

func newWhiskExecutor(functionName string) *whiskExecutor {
	ctx := context.Background()

	var address *string = nil
	if viper.GetBool("callback") {
		addr := ""
		var err error
		if viper.IsSet("ip") {
			addr = viper.GetString("ip")
		} else {
			addr, err = selectIP()
			if err != nil {
				panic(err)
			}
		}
		address = &addr
	}

	config := corwhisk2.WhiskClientConfig{
		RequestPerMinute:       viper.GetInt64("requestPerMinute"),
		ConcurrencyLimit:       viper.GetInt("requestBurstRate"),
		Host:                   viper.GetString("whiskHost"),
		Token:                  viper.GetString("whiskToken"),
		Context:                ctx,
		BatchRequestFeature:    viper.GetBool("eventBatching"),
		MultiDeploymentFeature: viper.GetBool("multiDeploy"),
		WriteMetrics:           viper.GetBool("verbose"),
		Address:                address,
	}

	return &whiskExecutor{
		WhiskClientApi: corwhisk2.NewWhiskClient(config),
		functionName:   functionName,
	}
}

//Implement the action loop that we trigger in the runtime
//this implements a process execution using system in and out...
//this is a modified version of https://github.com/apache/openwhisk-runtime-go/blob/master/examples/standalone/exec.go
func loop() {
	//register LCH
	capture := make(chan os.Signal, 2)
	signal.Notify(capture, syscall.SIGINT, syscall.SIGABRT, syscall.SIGUSR1, syscall.SIGUSR2)

	var logBuffer bytes.Buffer
	logFile := io.Writer(&logBuffer)
	log.SetOutput(logFile)
	//log.Printf("ACTION ENV: %v", os.Environ())

	// assign the main function
	type Action func(event task) (taskResult, error)
	var action Action
	action = handleWhsikRequest

	// input
	out := os.NewFile(3, "pipe")
	defer out.Close()
	reader := bufio.NewReader(os.Stdin)

	go func() {
		for {
			sig := <-capture
			switch sig {
			case syscall.SIGINT:
				handleWhiskPause(out)
			case syscall.SIGABRT:
				handleWhiskStop(out)
				logRemote(logBuffer.String())
				//! otherwise we will never terminate if requested..
				return
			case syscall.SIGUSR1:
				handleWhiskHint(out)
			case syscall.SIGUSR2:
				handleWhiskFreshen(out)
			}
		}
	}()

	// read-eval-print loop
	//log.Println("started")

	// send ack
	// note that it depends on the runtime,
	// go 1.13+ requires an ack, past versions does not
	fmt.Fprintf(out, `{ "ok": true, "pause":true,"finish":true,"hint":true,"freshen":true'}%s`, "\n")
	for {
		// read one line
		inbuf, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			break
		}

		log.Printf(">>>'%s'>>>", inbuf)

		// parse one line
		var input map[string]interface{}
		err = json.Unmarshal(inbuf, &input)
		if err != nil {
			handleError(err, out, logBuffer.String())
			continue
		}

		log.Printf("%v\n", input)

		// set environment variables
		err = json.Unmarshal(inbuf, &input)
		for k, v := range input {
			if k == "value" {
				continue
			}
			if s, ok := v.(string); ok {
				os.Setenv("__OW_"+strings.ToUpper(k), s)
			}
		}

		var invocation corwhisk2.WhiskPayload
		var payload task
		//Manage input parsing...
		if value, ok := input["value"].(map[string]interface{}); ok {
			buffer, _ := json.Marshal(value)
			err = json.Unmarshal(buffer, &invocation)
			if err != nil {
				handleError(err, out, logBuffer.String())
				continue
			}
			buffer, _ = json.Marshal(invocation.Value)
			err = json.Unmarshal(buffer, &payload)
			if err != nil {

				continue
			}
			for k, v := range invocation.Env {
				if v != nil {
					os.Setenv("__OW_"+strings.ToUpper(k), *v)
				}
			}
		}

		// process the request
		result, err := action(payload)

		if err != nil {
			handleError(err, out, logBuffer.String())
			continue
		}
		logString := logBuffer.String()
		logRemote(logString)
		if log.IsLevelEnabled(log.DebugLevel) {
			result.Log = logString
		}

		// encode the answer
		output, err := json.Marshal(&result)
		if err != nil {
			//log.Println(err.Error())
			fmt.Fprintf(out, "{ error: %q}\n", err.Error())
			continue
		}
		output = bytes.Replace(output, []byte("\n"), []byte(""), -1)

		log.Printf("'<<<%s'<<<", output)
		f, err := os.OpenFile("/tmp/activation.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			f.Write([]byte(result.Log))
			f.Close()
		}
		fmt.Fprintf(out, "%s\n", output)
	}
}

func handleError(err error, out *os.File, logString string) {
	log.Println(err.Error())
	fmt.Fprintf(out, "{ error: %q}\n", err.Error())
	logRemote(logString)
}

func logRemote(logString string) {
	rlh := os.Getenv("__OW_RemoteLoggingHost")
	if rlh != "" {
		go func(host string) {
			_, _ = http.Post(host, "text/plain", strings.NewReader(logString))
		}(rlh)
	}
}

func (l *whiskExecutor) HintSplits(splits uint) error {
	//TODO: call api hint
	return nil
}

func (l *whiskExecutor) Start(d *Driver) {
	whiskDriver = d
	for {
		//can't stop won't stop
		loop()
	}
}

func prepareWhiskResult(payload io.ReadCloser) taskResult {
	var result taskResult
	data, err := ioutil.ReadAll(payload)
	if err != nil {
		log.Errorf("%s", err)
	}

	err = json.Unmarshal(data, &result)
	if err != nil {
		log.Errorf("%s", err)
	}
	return result
}

func (l *whiskExecutor) BatchRunMapper(job *Job, jobNumber int, inputSplits [][]InputSplit) error {
	if !viper.IsSet("eventBatching") || !viper.IsSet("callback") {
		return fmt.Errorf("can't use batch reducer if eventBatching and callback flags are not set")
	}
	tasks := make([]task, 0)
	for binID, bin := range inputSplits {
		tasks = append(tasks, task{
			JobNumber:        jobNumber,
			Phase:            MapPhase,
			BinID:            uint(binID),
			Splits:           bin,
			IntermediateBins: job.intermediateBins,
			FileSystemType:   io2.FilesystemType(job.fileSystem),
			WorkingLocation:  job.outputPath,
		})
	}

	resp, err := l.InvokeBatch(l.functionName, tasks)
	if err != nil {
		log.Warnf("invocation failed with err:%+v", err)
		return err
	}

	batchResponses, err := l.WaitForBatch(resp)

	if err != nil {
		return err
	}

	for _, taskResult := range batchResponses {
		job.Collect(taskResult)
		atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
		atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))
	}

	return nil

}

func (l *whiskExecutor) BatchRunReducer(job *Job, jobNumber int, bins []uint) error {
	if !viper.IsSet("callback") {
		return fmt.Errorf("can't use batch reducer if eventBatching and callback flags are not set")
	}
	tasks := make([]task, 0)
	for _, binID := range bins {
		tasks = append(tasks, task{
			JobNumber:       jobNumber,
			Phase:           ReducePhase,
			BinID:           binID,
			FileSystemType:  io2.FilesystemType(job.fileSystem),
			WorkingLocation: job.outputPath,
			Cleanup:         job.config.Cleanup,
		})
	}

	//TODO:
	resp, err := l.InvokeBatch(l.functionName, tasks)
	if err != nil {
		log.Warnf("invocation failed with err:%+v", err)
		return err
	}

	batchResponses, err := l.WaitForBatch(resp)

	for _, taskResult := range batchResponses {
		job.Collect(taskResult)
		atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
		atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))
	}

	if err != nil {
		return err
	}
	return nil
}

func (l *whiskExecutor) RunMapper(job *Job, jobNumber int, binID uint, inputSplits []InputSplit) error {
	mapTask := task{
		JobNumber:        jobNumber,
		Phase:            MapPhase,
		BinID:            binID,
		Splits:           inputSplits,
		IntermediateBins: job.intermediateBins,
		FileSystemType:   io2.FilesystemType(job.fileSystem),
		WorkingLocation:  job.outputPath,
	}

	resp, err := l.Invoke(l.functionName, mapTask)
	if err != nil {
		log.Warnf("invocation failed with err:%+v", err)
		return err
	}

	taskResult := prepareWhiskResult(resp)
	job.Collect(taskResult)
	atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
	atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))

	return err
}

func (l *whiskExecutor) RunReducer(job *Job, jobNumber int, binID uint) error {
	mapTask := task{
		JobNumber:       jobNumber,
		Phase:           ReducePhase,
		BinID:           binID,
		FileSystemType:  io2.FilesystemType(job.fileSystem),
		WorkingLocation: job.outputPath,
		Cleanup:         job.config.Cleanup,
	}
	resp, err := l.Invoke(l.functionName, mapTask)
	if err != nil {
		log.Warnf("invocation failed with err:%+v", err)
		return err
	}

	taskResult := prepareWhiskResult(resp)
	job.Collect(taskResult)
	atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
	atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))

	return err
}

func (l *whiskExecutor) Deploy(driver *Driver) error {
	conf := corwhisk2.WhiskFunctionConfig{
		FunctionName: l.functionName,
		Memory:       viper.GetInt("lambdaMemory"),
		Timeout:      viper.GetInt("lambdaTimeout") * 1000,
	}

	if driver.cache != nil {
		conf.CacheConfigInjector = driver.cache.FunctionInjector()
	}

	err := l.WhiskClientApi.DeployFunction(conf)

	if err != nil {
		log.Infof("failed to deploy %s - %+v", l.functionName, err)
	}

	return err
}

func (l *whiskExecutor) Undeploy() error {
	err := l.WhiskClientApi.DeleteFunction(l.functionName)
	if err != nil {
		log.Infof("failed to remove function %+v", err)
	}

	return err
}

func (l *whiskExecutor) WaitForBatch(resp []interface{}) ([]taskResult, error) {
	set := NewConcurrentSet()
	set.AddAll(resp)

	results := l.ReceiveUntil(func() bool {
		return set.IsEmpty()
	})

	taskResults := make([]taskResult, 0)
	ctx, _ := context.WithTimeout(context.Background(), 15*time.Minute)
	for {
		select {
		case resp, ok := <-results:
			if !ok {
				return taskResults, nil
			} else {
				taskResult := prepareWhiskResult(resp)
				taskResults = append(taskResults, taskResult)
				set.Remove(taskResult.RId)
			}
		case <-ctx.Done():
			return taskResults, fmt.Errorf("timed out to wait for BatchResults")
		}
	}
}

func (l *whiskExecutor) InvokeBatch(functionName string, tasks []task) ([]interface{}, error) {
	errors := make([]error, 0)
	iids := make([]interface{}, 0)

	if !viper.IsSet("eventBatching") {
		log.Info("batching Events using async")

		for _, t := range tasks {
			iid, err := l.InvokeAsync(l.functionName, t)
			if err != nil {
				log.Debugf("failed to send [j:%d|p:%d,b:%d] - %+v", t.JobNumber, t.Phase, t.BinID, err)
				errors = append(errors, err)
			}
			if iid != nil && iid.(string) != "" {
				iids = append(iids, iid.(string))
			} else {
				log.Debugf("no invocation id for [j:%d|p:%d,b:%d]", t.JobNumber, t.Phase, t.BinID)
				errors = append(errors, fmt.Errorf("missing invocation references"))
			}
		}

	} else {
		batchSize := viper.GetInt("eventBatchSize")
		for i := 0; i < len(tasks); i += batchSize {
			end := i + batchSize

			if end > len(tasks) {
				end = len(tasks)
			}

			task := tasks[i:end]
			taskBatch := make([]interface{}, len(task))
			for i := range task {
				taskBatch[i] = task[i]
			}
			ids, err := l.InvokeAsBatch(l.functionName, taskBatch)
			if err != nil {
				errors = append(errors, err)
			}
			iids = append(iids, ids...)
		}
	}

	if len(errors) > 0 {
		return iids, fmt.Errorf("%d/%d errors during batch invocation", len(errors), len(tasks))
	} else {
		return iids, nil
	}
}
