package polling

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

type DNNModelPolling struct {
	PollLogger

	PolledTasks    map[string]bool
	backoffCounter map[string]int

	features map[string]JobFeatures

	PredictionRequest
}

type JobFeatures struct {
	number_of_jobs         float64
	prev_job_bytes_written float64
	splits                 float64
	split_size             float64
	map_bin_size           float64
	reduce_bin_size        float64
	max_concurrency        float64
}

func (b *DNNModelPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	predictionStartTime := time.Now().UnixNano()

	var backoff int
	timebuffer := 2

	b.PrematurePollMutex.Lock()
	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}
	b.PrematurePollMutex.Unlock()

	b.PolledTaskMutex.Lock()
	if b.PolledTasks == nil {
		b.PolledTasks = make(map[string]bool)
	}

	if present, ok := b.PolledTasks[RId]; ok && present {
		b.PolledTaskMutex.Unlock()
		//we dont want to use the model again... fall back to constant polling
		backoff = 10
		log.Debugf("Poll fallback backoff %s for %d seconds", RId, backoff)

	} else {

		//get the prediction

		var sum int
		for _, val := range b.ExecutionTimes {
			sum += val
		}

		b.IdMapMutex.Lock()
		jobId := b.rIdToJobId[RId]
		taskId := b.rIdToTaskId[RId]
		b.IdMapMutex.Unlock()

		var number_of_inputs, bin_size, Map, Reduce float64

		b.TaskMapMutex.Lock()
		number_of_inputs = float64(b.taskInfos[taskId].NumberOfInputs)
		bin_size = float64(b.taskInfos[taskId].BinSize)
		if b.taskInfos[taskId].Phase == 0 {
			Map = 1
			Reduce = 0
		} else {
			Map = 0
			Reduce = 1
		}
		b.TaskMapMutex.Unlock()

		b.JobMapMutex.Lock()
		if b.features == nil {
			b.features = make(map[string]JobFeatures)
		}
		if _, ok := b.features[RId]; !ok {
			var entry JobFeatures
			entry.number_of_jobs = float64(b.jobInfos[jobId].NumberOfJobs)
			entry.prev_job_bytes_written = float64(b.jobInfos[jobId].PrevJobBytesWritten)
			entry.splits = float64(b.jobInfos[jobId].Splits)
			entry.split_size = float64(b.jobInfos[jobId].SplitSize)
			entry.map_bin_size = float64(b.jobInfos[jobId].MapBinSize)
			entry.reduce_bin_size = float64(b.jobInfos[jobId].ReduceBinSize)
			entry.max_concurrency = float64(b.jobInfos[jobId].MaxConcurrency)
			b.features[RId] = entry
		}
		f := b.features[RId]
		b.JobMapMutex.Unlock()

		log.Debug(fmt.Sprint(f.number_of_jobs, f.prev_job_bytes_written, f.splits, f.split_size, f.map_bin_size, f.reduce_bin_size, f.max_concurrency, number_of_inputs, bin_size, Map, Reduce))
		data := []float64{f.number_of_jobs, f.prev_job_bytes_written, f.splits, f.split_size, f.map_bin_size, f.reduce_bin_size, f.max_concurrency, number_of_inputs, bin_size, Map, Reduce}
		prediction, err := b.GetPrediction("pollingDNN", data)
		if err != nil {
			backoff = 16
		} else {
			backoff = prediction + timebuffer
		}
		if backoff <= 0 {
			backoff = 10
		}

		b.PolledTasks[RId] = true
		b.PolledTaskMutex.Unlock()
		log.Debugf("Poll predicted backoff %s for %d seconds", RId, backoff)
	}

	predictionEndTime := time.Now().UnixNano()
	b.PollPredictionTimeMutex.Lock()
	if _, ok := b.PollPredictionTimes[RId]; ok {
		b.PollPredictionTimes[RId] += (predictionEndTime - predictionStartTime)
	} else {
		b.PollPredictionTimes[RId] = (predictionEndTime - predictionStartTime)
	}
	b.PollPredictionTimeMutex.Unlock()

	channel := make(chan interface{})
	go func() {
		select {
		case <-context.Done():
			channel <- struct{}{}
		case <-time.After(time.Second * time.Duration(backoff)):
			channel <- struct{}{}
		}
	}()
	return channel, nil
}
