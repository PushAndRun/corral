package polling

import (
	"context"

	"time"

	log "github.com/sirupsen/logrus"
)

type AveragePolling struct {
	PollLogger

	PolledTasks    map[string]bool
	backoffCounter map[string]int
}

func (b *AveragePolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
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
		//we dont want to use the average again... fall back to dup.-backoff
		if b.backoffCounter == nil {
			b.backoffCounter = make(map[string]int)
		}

		if last, ok := b.backoffCounter[RId]; ok {
			b.backoffCounter[RId] = last + last
			backoff = last
		} else {
			backoff = 16
			b.backoffCounter[RId] = backoff
		}
		log.Println("Use the fallback")

	} else {
		b.PolledTaskMutex.Unlock()
		//get the average

		var sum int
		for _, val := range b.ExecutionTimes {
			sum += val
		}
		backoff = int(sum/len(b.ExecutionTimes)) + timebuffer
		log.Println("Use the average")
		b.PolledTasks[RId] = true

	}

	predictionEndTime := time.Now().UnixNano()
	b.PollPredictionTimeMutex.Lock()
	if _, ok := b.PollPredictionTimes[RId]; ok {
		b.PollPredictionTimes[RId] += (predictionEndTime - predictionStartTime)
	} else {
		b.PollPredictionTimes[RId] = (predictionEndTime - predictionStartTime)
	}
	b.PollPredictionTimeMutex.Unlock()
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
