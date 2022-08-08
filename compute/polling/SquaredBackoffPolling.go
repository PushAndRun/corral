package polling

import (
	"context"
	"time"

	"github.com/ISE-SMILE/corral/api"

	log "github.com/sirupsen/logrus"
)

type SquaredBackoffPolling struct {
	backoffCounter map[string]int
	PollLogger
}

func (b *SquaredBackoffPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	predictionStartTime := time.Now().UnixNano()
	var backoff int
	offset := 2
	slope := 2

	if b.backoffCounter == nil {
		b.backoffCounter = make(map[string]int)
	}

	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}

	backoff = slope*(b.NumberOfPrematurePolls[RId]*b.NumberOfPrematurePolls[RId]) + offset

	predictionEndTime := time.Now().UnixNano()

	if _, ok := b.PollPredictionTimes[RId]; ok {
		b.PollPredictionTimes[RId] += (predictionEndTime - predictionStartTime)
	} else {
		b.PollPredictionTimes[RId] = (predictionEndTime - predictionStartTime)
	}

	log.Debugf("Poll backoff %s for %d seconds", RId, backoff)
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

func (b *SquaredBackoffPolling) TaskUpdate(info api.TaskInfo) error {
	if info.Failed || info.Completed {
		delete(b.backoffCounter, info.RId)
		return b.PollLogger.TaskUpdate(info)
	} else {
		return b.PollLogger.TaskUpdate(info)
	}
}
