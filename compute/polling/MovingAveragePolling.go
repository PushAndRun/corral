package polling

import (
	"context"

	"time"

	log "github.com/sirupsen/logrus"
)

type MovingAveragePolling struct {
	PollLogger
	ExecutionTimes []int
	PolledTasks    map[string]bool
	backoffCounter map[string]int
}

func (b *MovingAveragePolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	predictionStartTime := time.Now().UnixNano()

	var backoff int
	timebuffer := 2

	b.MapMutex.Lock()
	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}
	b.MapMutex.Unlock()

	if b.PolledTasks == nil {
		b.PolledTasks = make(map[string]bool)
	}

	if _, ok := b.PolledTasks[RId]; !ok {
		if b.ExecutionTimes == nil {
			b.ExecutionTimes = make([]int, 5)
			for x := range b.ExecutionTimes {
				b.ExecutionTimes[x] = 5
			}
		}
		var sum int
		for _, val := range b.ExecutionTimes {
			sum += val
		}

		backoff = (sum / len(b.ExecutionTimes)) + timebuffer
		b.PolledTasks[RId] = true

	} else {
		if b.backoffCounter == nil {
			b.backoffCounter = make(map[string]int)
		}

		if last, ok := b.backoffCounter[RId]; ok {
			b.backoffCounter[RId] = last * last
			backoff = last
		} else {
			backoff = 2
			b.backoffCounter[RId] = backoff
		}

	}

	predictionEndTime := time.Now().UnixNano()

	if _, ok := b.PollPredictionTimes[RId]; ok {
		b.PollPredictionTimes[RId] += (predictionEndTime - predictionStartTime)
	} else {
		b.PollPredictionTimes[RId] = (predictionEndTime - predictionStartTime)
	}

	log.Debugf("Poll (average) backoff %s for %d seconds", RId, backoff)

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

func (b *MovingAveragePolling) SetFinalPollTime(RId string, timeNano int64) {

	b.PollLogger.SetFinalPollTime(RId, timeNano)

	var startTime int64
	b.TaskMapMutex.Lock()
	if b.taskInfos != nil {
		for _, val := range b.taskInfos {
			if val.RId == RId {
				startTime = val.RequestStart.UnixNano()
			}
		}
		b.TaskMapMutex.Unlock()

		predictionStartTime := time.Now().UnixNano()

		for i := range b.ExecutionTimes {
			if i < len(b.ExecutionTimes)-1 {
				b.ExecutionTimes[i+1] = b.ExecutionTimes[i]
			}
		}

		b.ExecutionTimes[0] = int(time.Duration(timeNano-startTime) / time.Second)

		predictionEndTime := time.Now().UnixNano()

		if _, ok := b.PollPredictionTimes[RId]; ok {
			b.PollPredictionTimes[RId] += (predictionEndTime - predictionStartTime)
		} else {
			b.PollPredictionTimes[RId] = (predictionEndTime - predictionStartTime)
		}
	}

}
