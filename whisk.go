package corral

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"time"

	"runtime/debug"

	"github.com/kjk/betterguid"

	api "github.com/ISE-SMILE/corral/api"
	. "github.com/ISE-SMILE/corral/compute/corwhisk"
	"github.com/ISE-SMILE/corral/internal/corcache"
	fs "github.com/ISE-SMILE/corral/internal/corfs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	whiskDriver *Driver
)

func runningInWhisk() bool {
	expectedEnvVars := []string{"__OW_EXECUTION_ENV", "__OW_ACTIVATION_ID"}
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

	if name := os.Getenv("NODENAME"); name != "" {
		whiskNodeName = name
		return whiskNodeName
	}

	//is only present in some upstream openwhisk builds ;)
	if _, err := os.Stat("/vmhost"); err == nil {
		data, err := ioutil.ReadFile("/vmhost")
		if err == nil {
			whiskNodeName = string(bytes.Replace(data, []byte("\n"), []byte(""), -1))
			return whiskNodeName
		}
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
	return os.Getenv("__OW_ACTIVATION_ID")
}

func handleWhsikRequest(task api.Task) (api.TaskResult, error) {
	return handle(whiskDriver, whiskHostID, whiskRequestID)(task)
}

func handleWhiskHook(out io.Writer, hook func(*Job) string) {
	if whiskDriver != nil {
		job := whiskDriver.CurrentJob()
		if job != nil {
			msg := hook(job)
			if msg != "" {
				_, _ = fmt.Fprintln(out, msg)
			}
		}
	}
}

func handleWhiskPause(out io.Writer) {
	handleWhiskHook(out, func(job *Job) string {
		if job != nil && job.PauseFunc != nil {
			return job.PauseFunc()
		} else {
			return ""
		}
	})
}

func handleWhiskStop(out io.Writer) {
	handleWhiskHook(out, func(job *Job) string {
		if job != nil && job.StopFunc != nil {
			return job.StopFunc()
		} else {
			return ""
		}
	})
}

func handleWhiskFreshen(out io.Writer) {

}

func handleWhiskHint(out io.Writer) {
	handleWhiskHook(out, func(job *Job) string {
		return job.HintFunc()
	})
}

type whiskExecutor struct {
	WhiskClientApi
	functionName string
	polling      api.PollingStrategy
}

func newWhiskExecutor(functionName string, polling api.PollingStrategy) *whiskExecutor {
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

	config := WhiskClientConfig{
		RequestPerMinute:       viper.GetInt64("requestPerMinute"),
		ConcurrencyLimit:       viper.GetInt("requestBurstRate"),
		Host:                   viper.GetString("whiskHost"),
		Token:                  viper.GetString("whiskToken"),
		Context:                ctx,
		BatchRequestFeature:    viper.GetBool("eventBatching"),
		MultiDeploymentFeature: viper.GetBool("multiDeploy"),
		WriteMetrics:           viper.GetBool("verbose"),
		Address:                address,
		RemoteLoggingHost:      viper.GetString("remoteLoggingHost"),
		Polling:                polling,
	}

	return &whiskExecutor{
		WhiskClientApi: NewWhiskClient(config),
		functionName:   functionName,
		polling:        polling,
	}
}

//Implement the action loop that we trigger in the runtime
//this implements a process execution using system in and out...
//this is a modified version of https://github.com/apache/openwhisk-runtime-go/blob/master/examples/standalone/exec.go
func loop(ack string) {

	var logBuffer bytes.Buffer
	logFile := io.Writer(&logBuffer)
	log.SetOutput(logFile)
	//log.Printf("ACTION ENV: %v", os.Environ())

	// assign the main function
	type Action func(event api.Task) (api.TaskResult, error)
	var action Action
	action = handleWhsikRequest

	// input
	var out *os.File

	//register LCH
	whiskActivateHooks(out)

	if os.Getenv("MOCK") != "" {
		out = os.Stdout
	} else {
		out = os.NewFile(3, "pipe")
	}
	defer out.Close()
	reader := bufio.NewReader(os.Stdin)

	// read-eval-print loop

	// send ack
	// note that it depends on the runtime,
	// go 1.13+ requires an ack, past versions does not
	fmt.Fprintf(out, `%s%s`, ack, "\n")
	_ = out.Sync()
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred:", err)
			handleError(fmt.Errorf("panic %+v", err), out, logBuffer.String())
		}
	}()
	for {
		// read one line
		//XXX: this will fail for realy long lines >> 65k
		inbuf, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			break
		}
		//log.Printf(">>>'%s'>>>", inbuf)

		// parse one line
		var input map[string]interface{}
		err = json.Unmarshal(inbuf, &input)
		if err != nil {
			handleError(err, out, logBuffer.String())
			continue
		}

		//log.Printf("%v\n", input)

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

		var invocation WhiskPayload
		var payload api.Task
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
				handleError(err, out, logBuffer.String())
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
		//if log.IsLevelEnabled(log.DebugLevel) {
		//	result.Log = logString
		//}

		// encode the answer
		output, err := json.Marshal(&result)
		if err != nil {
			handleError(err, out, logBuffer.String())
			continue
		}
		output = bytes.Replace(output, []byte("\n"), []byte(""), -1)

		//log.Printf("'<<<%s'<<<", output)

		fmt.Fprintf(out, "%s\n", output)
		_ = out.Sync()
		//attemt to free some memory
		go func() {
			debug.FreeOSMemory()
		}()
	}
}

func handleError(err error, out *os.File, logString string) {
	log.Printf("error: %+v", err)
	fmt.Fprintf(out, "{ \"error\": \"%s\",\"stack\":\"%s\"}\n",
		strings.ReplaceAll(html.EscapeString(fmt.Sprintf("%+v", err)), "\n", " "),
		strings.ReplaceAll(html.EscapeString(string(debug.Stack())), "\n", " "))
	logRemote(logString)
}

func logRemote(logString string) {
	rlh := os.Getenv("__OW_RemoteLoggingHost")
	if rlh != "" {
		go func(host string) {
			_, _ = http.Post(host, "text/plain", strings.NewReader(logString))
		}(rlh)
	}
	if viper.GetBool("veryverbose") {
		f, err := os.OpenFile("activation.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			f.Write([]byte(logString))
			f.Close()
		}
	}
}

type WhiskAckMsg struct {
	Ok        bool `json:"ok"`
	Pausing   bool `json:"pause,omitempty"`
	Finishing bool `json:"finish,omitempty"`
	Hinting   bool `json:"hint,omitempty"`
	Freshen   bool `json:"freshen,omitempty"`
}

func checkJobHooks(d *Driver) string {
	msg := &WhiskAckMsg{
		Ok:      true,
		Hinting: true,
	}
	for _, job := range d.jobs {
		if job.PauseFunc != nil {
			msg.Pausing = true
		}
		if job.StopFunc != nil {
			msg.Finishing = true
		}
		if job.HintFunc != nil {
			msg.Hinting = true
		}
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Debugf("failed to mashal WhiskAckMsg %+v", err)
		return `{"ok":true}`
	} else {
		return string(data)
	}
}

func (l *whiskExecutor) prepareWhiskResult(payload io.ReadCloser) (api.TaskResult, error) {
	var result api.TaskResult
	data, err := ioutil.ReadAll(payload)
	if err != nil {
		return api.TaskResult{}, err
	}

	log.Debugf("got %s", string(data))

	err = json.Unmarshal(data, &result)
	if err != nil {
		return api.TaskResult{}, err
	}
	//Task.JobNumber, Task.Phase, Task.BinID),

	//ids := parseJId(result)

	_ = l.polling.TaskUpdate(api.TaskInfo{
		RId:    result.RId,
		TaskId: result.TaskId,
		JobId:  result.JobId,
		//JobNumber:         ids[0],
		//BinId:             ids[2],
		//Phase:             ids[1],
		RequestCompletedAndPolled: time.Now(),
		FunctionExecutionStart:    result.EStart,
		FunctionExecutionEnd:      result.EEnd,
		FunctionExecutionDuration: int64(time.Duration(result.EEnd)*time.Nanosecond - time.Duration(result.EStart)*time.Nanosecond),
		RuntimeId:                 result.CId,
		Completed:                 true,
		Failed:                    false,
	})

	return result, nil
}

func parseJId(result api.TaskResult) []int {
	var ids []int
	for _, x := range strings.Split(result.JId, "_") {
		i, _ := strconv.Atoi(x)
		ids = append(ids, i)
	}
	return ids
}

func (l *whiskExecutor) Start(d *Driver) {
	whiskDriver = d

	msg := checkJobHooks(d)

	for {
		//can't stop won't stop
		loop(msg)
	}
}

type activation struct {
	ActivationId string `json:"activationId"`
	Code         int    `json:"statusCode"`
	Response     struct {
		Result api.TaskResult `json:"result"`
	} `json:"response"`
}

type WhiskInvokationError struct {
	invocations map[string]api.Task
}

//HintSplits perform splits*Hint invocations calling the JobHintFunction
func (l *whiskExecutor) HintSplits(splits uint) error {
	var hint int = int(splits)
	_, err := l.Hint(l.functionName, nil, &hint)
	return err
}

func (l *whiskExecutor) BatchRunMapper(job *Job, jobNumber int, inputSplits [][]api.InputSplit) error {
	if !viper.IsSet("eventBatching") || !viper.IsSet("callback") {
		return fmt.Errorf("can't use batch reducer if eventBatching and callback flags are not set")
	}

	tasks := make([]api.Task, 0)
	for binID, bin := range inputSplits {

		tasks = append(tasks, api.Task{
			JobId:            job.JobId,
			JobNumber:        jobNumber,
			TaskId:           betterguid.New(),
			Phase:            api.MapPhase,
			BinID:            uint(binID),
			Splits:           bin,
			IntermediateBins: job.intermediateBins,
			FileSystemType:   fs.FilesystemType(job.fileSystem),
			CacheSystemType:  corcache.CacheSystemTypes(job.cacheSystem),
			WorkingLocation:  job.outputPath,
		})
	}
	collector := make(chan error)
	l.invokeBatch(job, tasks, collector)
	return <-collector
}

func (l *whiskExecutor) BatchRunReducer(job *Job, jobNumber int, bins []uint) error {
	if !viper.IsSet("eventBatching") || !viper.IsSet("callback") {
		return fmt.Errorf("can't use batch reducer if eventBatching and callback flags are not set")
	}
	tasks := make([]api.Task, 0)
	for _, binID := range bins {
		tasks = append(tasks, api.Task{
			JobId:           job.JobId,
			TaskId:          betterguid.New(),
			JobNumber:       jobNumber,
			Phase:           api.ReducePhase,
			BinID:           binID,
			FileSystemType:  fs.FilesystemType(job.fileSystem),
			WorkingLocation: job.outputPath,
			CacheSystemType: corcache.CacheSystemTypes(job.cacheSystem),
			Cleanup:         job.config.Cleanup,
		})
	}

	collector := make(chan error)
	l.invokeBatch(job, tasks, collector)
	return <-collector
}

func (l *whiskExecutor) RunMapper(job *Job, jobNumber int, binID uint, inputSplits []api.InputSplit) error {
	mapTask := api.Task{
		TaskId:           betterguid.New(),
		JobId:            job.JobId,
		JobNumber:        jobNumber,
		Phase:            api.MapPhase,
		BinID:            binID,
		Splits:           inputSplits,
		IntermediateBins: job.intermediateBins,
		FileSystemType:   fs.FilesystemType(job.fileSystem),
		CacheSystemType:  corcache.CacheSystemTypes(job.cacheSystem),
		WorkingLocation:  job.outputPath,
	}

	taskResult, err := l.invoke(mapTask)
	if err == nil {
		job.Collect(taskResult)
		atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
		atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))
	}
	return err
}

func (l *whiskExecutor) RunReducer(job *Job, jobNumber int, binID uint) error {
	mapTask := api.Task{
		TaskId:          betterguid.New(),
		JobId:           job.JobId,
		JobNumber:       jobNumber,
		Phase:           api.ReducePhase,
		BinID:           binID,
		FileSystemType:  fs.FilesystemType(job.fileSystem),
		CacheSystemType: corcache.CacheSystemTypes(job.cacheSystem),
		WorkingLocation: job.outputPath,
		Cleanup:         job.config.Cleanup,
	}

	taskResult, err := l.invoke(mapTask)
	if err == nil {
		job.Collect(taskResult)
		atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
		atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))
	}

	return err
}

func (l *whiskExecutor) invoke(mapTask api.Task) (api.TaskResult, error) {
	inputs := -1
	if mapTask.Splits != nil {
		inputs = len(mapTask.Splits)
	}

	_ = l.polling.TaskUpdate(api.TaskInfo{
		JobId:                  mapTask.JobId,
		TaskId:                 mapTask.TaskId,
		BinId:                  int(mapTask.BinID),
		Phase:                  int(mapTask.Phase),
		JobNumber:              int(mapTask.JobNumber),
		RequestStart:           time.Now(),
		NumberOfInputs:         inputs,
		NumberOfPrematurePolls: -1,
	})
	resp, err := l.Invoke(l.functionName, mapTask)
	if err != nil {
		log.Warnf("invocation failed with err:%+v", err)
		_ = l.polling.TaskUpdate(api.TaskInfo{
			TaskId:                    mapTask.TaskId,
			JobId:                     mapTask.JobId,
			BinId:                     int(mapTask.BinID),
			Phase:                     int(mapTask.Phase),
			RequestCompletedAndPolled: time.Now(),
			Failed:                    true,
		})
		return api.TaskResult{}, err
	}

	taskResult, err := l.prepareWhiskResult(resp)
	if err != nil {
		log.Debugf("failed to read result from whisk:%+v", err)
	}
	return taskResult, nil
}

func (l *whiskExecutor) Deploy(driver *Driver) error {
	conf := WhiskFunctionConfig{
		FunctionName: l.functionName,
		Memory:       viper.GetInt("lambdaMemory"),
		Timeout:      viper.GetInt("lambdaTimeout") * 1000,
	}

	if driver.cache != nil {
		log.Debug("adding cache injector")
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

func (e WhiskInvokationError) Error() string {
	return fmt.Sprintf("activation(s) %+v failed", e.Activations())
}

func NewWhiskInvokationError() *WhiskInvokationError {
	return &WhiskInvokationError{
		make(map[string]api.Task),
	}
}

func (e *WhiskInvokationError) Add(activationId string, t api.Task) {
	e.invocations[activationId] = t
}

func (e *WhiskInvokationError) Activations() []string {
	keys := make([]string, len(e.invocations))

	i := 0
	for k := range e.invocations {
		keys[i] = k
		i++
	}
	return keys
}

func (e *WhiskInvokationError) FailedTasks() []api.Task {
	keys := make([]api.Task, len(e.invocations))

	i := 0
	for _, v := range e.invocations {
		keys[i] = v
		i++
	}
	return keys
}

func (e *WhiskInvokationError) isEmpty() bool {
	return len(e.invocations) <= 0
}

func (l *whiskExecutor) InvokeBatch(functionName string, tasks []api.Task, activationSet *ActivationSet) error {
	errors := make([]error, 0)

	if !viper.IsSet("eventBatching") || len(tasks) < 4 {
		log.Info("batching Events using async")

		for _, t := range tasks {
			iid, err := l.InvokeAsync(functionName, t)

			if err != nil {
				log.Debugf("failed to send [j:%d|p:%d,b:%d] - %+v", t.JobNumber, t.Phase, t.BinID, err)
				errors = append(errors, err)
			}
			if iid != nil && iid.(string) != "" {
				activationSet.AddWithData(iid.(string), t)
				l.polling.TaskUpdate(api.TaskInfo{
					TaskId:    t.TaskId,
					JobId:     t.JobId,
					JobNumber: t.JobNumber,
					BinId:     int(t.BinID),
					Phase:     int(t.Phase),
					RId:       iid.(string),
				})
				log.Debugf("got activation %+v", iid)
			} else {
				log.Debugf("no invocation id for [j:%d|p:%d,b:%d]", t.JobNumber, t.Phase, t.BinID)
				errors = append(errors, fmt.Errorf("missing invocation references"))
			}
		}
		activationSet.Close()

	} else {
		batchSize := viper.GetInt("eventBatchSize")
		if batchSize <= 0 {
			batchSize = 8
		}
		taskBatches := make([][]api.Task, 0)
		batch := 0
		for i := 0; i < len(tasks); i += batchSize {
			end := i + batchSize

			if end > len(tasks) {
				end = len(tasks)
			}

			task := tasks[i:end]
			taskBatch := make([]api.Task, len(task))
			for i := range task {
				taskBatch[i] = task[i]
			}
			taskBatches = append(taskBatches, taskBatch)
			batch++
		}
		var wg sync.WaitGroup
		wg.Add(len(taskBatches))
		for t := 0; t < len(taskBatches); t++ {
			go func(t int, batch []api.Task) {
				aIds, err := l.InvokeAsBatch(functionName, batch)
				log.Debugf("invoked batch %d with %d activations", t, len(aIds))
				if err != nil {
					log.Debugf("failed to send batch request %+v", err)
				} else {
					for i, id := range aIds {
						if s, ok := id.(string); ok {
							err := activationSet.AddWithData(s, batch[i])
							l.polling.TaskUpdate(api.TaskInfo{
								TaskId:    batch[i].TaskId,
								JobId:     batch[i].JobId,
								JobNumber: batch[i].JobNumber,
								BinId:     int(batch[i].BinID),
								Phase:     int(batch[i].Phase),
								RId:       s,
							})
							if err != nil {
								log.Errorf("failed to add activation id %s to set %+v", s, err)
								return
							}
						}
					}
				}
				wg.Done()
			}(t, taskBatches[t])
		}

		//Close Activation Set after we managed to send all batches
		go func() {
			wg.Wait()
			log.Debugf("send all batches")
			activationSet.Close()
		}()
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d/%d errors during batch invocation", len(errors), len(tasks))
	} else {
		return nil
	}
}

func (l *whiskExecutor) WaitForBatch(activations *ActivationSet) ([]api.TaskResult, error) {
	taskResults := make([]api.TaskResult, 0)
	invErr := NewWhiskInvokationError()

	//this is a small batch to wait for we rather just pull without all the fuss
	if activations.Drained(2) {
		log.Debugf("skipping wait for batch, already drained")

		polledResults, err := l.pollActivation(activations.List())
		activations.Clear()
		if err != nil {
			return nil, err
		}
		taskResults = append(taskResults, polledResults...)
		return taskResults, nil
	}

	//TODO: timeout should be configurable, 15 min is reasonable for now
	timeout := 15 * time.Minute
	chn := l.ReceiveUntil(func() bool {
		return activations.Drained(2)
	}, &timeout)

	batchWtime := time.Minute

	starvation := 0
	for {
		select {
		case in, ok := <-chn:
			if ok {
				starvation = 0
				data, err := ioutil.ReadAll(in)
				if err != nil {
					return nil, err
				}
				var activation activation
				err = json.Unmarshal(data, &activation)
				if err != nil {
					return nil, err
				}
				log.Debugf("recieved %+v", activation)
				if activation.Code == 0 {
					if taskResult := activation.Response.Result; taskResult.RId != "" {

						taskResults = append(taskResults, taskResult)

						elat := time.Duration(taskResult.EEnd-taskResult.EStart) * time.Nanosecond

						//ids := parseJId(taskResult)

						_ = l.polling.TaskUpdate(api.TaskInfo{
							JobId:  taskResult.JobId,
							TaskId: taskResult.TaskId,
							//JobNumber:         ids[0],
							//BinId:             ids[2],
							//Phase:             ids[1],
							//RequestReceived:           time.Now(),
							FunctionExecutionDuration: int64(elat),
							RuntimeId:                 "",
							Completed:                 true,
							RId:                       activation.ActivationId,
						})

						activations.Remove(taskResult.RId)
						api.TryCollect(map[string]interface{}{
							"RId":  activation.ActivationId,
							"rEnd": time.Now().UnixMilli(),
						})

						batchWtime += elat
						batchWtime /= 2
					} else {
						//TaskResult is nil
						log.Warnf("unexpected taskresult of %s is nil", activation.ActivationId)
					}

				} else {

					t := activations.Remove(activation.ActivationId)
					log.Debugf("failed %s atemting recovery %+v ", activation.ActivationId, t)
					//recoverable error
					if t != nil {
						invErr.Add(activation.ActivationId, t.(api.Task))
					} else {
						return taskResults, fmt.Errorf("unrecoverable error")
					}
				}
				if activations.IsEmpty() {
					return taskResults, invErr
				}
			} else {
				polledResults, err := l.pollActivation(activations.List())
				activations.Clear()
				if err != nil {
					return nil, err
				}
				taskResults = append(taskResults, polledResults...)
				return taskResults, nil
			}
		case <-time.After(batchWtime):

			if activations.Drained(5) {
				remaining := activations.List()
				activations.Clear()
				polledResults, err := l.pollActivation(remaining)

				if err != nil {
					return nil, err
				}
				taskResults = append(taskResults, polledResults...)
				return taskResults, nil
			} else {

				starvation += activations.Len()
				log.Debugf("starving %d", starvation)
				if starvation > 256 {
					starvation = 0
					//we will now pull the oldes two and mark them as failed
					timeouts := activations.Top(2)
					for _, id := range timeouts {
						t := activations.Remove(id)
						log.Debugf("killed %s atempting recovery %+v ", id, t)
						invErr.Add(id, t.(api.Task))
						task := t.(api.Task)
						l.polling.TaskUpdate(api.TaskInfo{
							TaskId:                    task.TaskId,
							JobId:                     task.JobId,
							JobNumber:                 task.JobNumber,
							BinId:                     int(task.BinID),
							Phase:                     int(task.Phase),
							RequestCompletedAndPolled: time.Now(),
							Failed:                    true,
						})
					}

				}
			}
		}
	}
}

func (l *whiskExecutor) pollActivation(remaining []string) ([]api.TaskResult, error) {
	taskResults := make([]api.TaskResult, 0)
	var fetchError error
	log.Debugf("callback timed out, polling remaining %d activations", len(remaining))
	for _, s := range remaining {
		activation, err := l.PollActivation(s)
		if err != nil {
			fetchError = err
			log.Debugf("failed to fetch activation %s - %+v", s, err)
			continue
		}
		taskResult, err := l.prepareWhiskResult(activation)
		if err != nil {
			fetchError = err
			log.Debugf("failed to process whisk result %s - %+v", s, err)
			continue
		}
		taskResults = append(taskResults, taskResult)

	}
	return taskResults, fetchError
}

func (l *whiskExecutor) invokeBatch(job *Job, tasks []api.Task, collector chan error) {
	activations := NewSet()

	go func() {
		batchResponses, err := l.WaitForBatch(activations)
		if err != nil {
			if inv, is := asWiskInvocationError(err); is {
				results, err := l.tryRecover(inv)
				if err != nil {
					collector <- err
					return
				}
				batchResponses = append(batchResponses, results...)
			} else {
				collector <- err
				return
			}

		}

		for _, taskResult := range batchResponses {
			job.Collect(taskResult)
			atomic.AddInt64(&job.bytesRead, int64(taskResult.BytesRead))
			atomic.AddInt64(&job.bytesWritten, int64(taskResult.BytesWritten))
		}
		log.Infof("completed batch")
		collector <- nil
	}()

	for _, t := range tasks {
		splits := -1
		if t.Splits != nil {
			splits = len(t.Splits)
		}
		l.polling.TaskUpdate(api.TaskInfo{
			TaskId:                    t.TaskId,
			JobId:                     t.JobId,
			JobNumber:                 t.JobNumber,
			BinId:                     int(t.BinID),
			Phase:                     int(t.Phase),
			RequestStart:              time.Now(),
			RequestCompletedAndPolled: time.Time{},
			NumberOfInputs:            splits,
		})
	}

	err := l.InvokeBatch(l.functionName, tasks, activations)
	if err != nil {
		log.Warnf("invocation failed with err:%+v", err)
		collector <- err
	}
}

func (l *whiskExecutor) tryRecover(inv *WhiskInvokationError) ([]api.TaskResult, error) {

	if !inv.isEmpty() {
		results := make([]api.TaskResult, 0)
		log.Debugf("attemt to recover %+v", inv)
		//attempt recovery
		toRecover := inv.FailedTasks()
		jobs := make([][]api.Task, 4)

		jobs[0] = make([]api.Task, 0)
		jobs[1] = make([]api.Task, 0)
		jobs[2] = make([]api.Task, 0)
		jobs[3] = make([]api.Task, 0)

		for i := 0; i < len(toRecover); i++ {
			jobs[i%4] = append(jobs[i%4], toRecover[i])
		}
		for _, j := range jobs {
			go func(todo []api.Task) {
				for _, t := range todo {
					resp, err := l.Invoke(l.functionName, t)
					if err != nil {
						log.Debugf("recovery failed with %+v", err)
						continue
					}
					result, err := l.prepareWhiskResult(resp)
					if err != nil {
						log.Debugf("recovery failed with %+v", err)
						continue
					}
					results = append(results, result)
				}
			}(j)
		}
		log.Debugf("recovered %+v", inv)
		return results, nil
	}
	return []api.TaskResult{}, nil
}

func asWiskInvocationError(err error) (*WhiskInvokationError, bool) {
	v, ok := err.(*WhiskInvokationError)
	return v, ok
}
