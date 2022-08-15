package polling

import (
	"context"
	"time"

	"github.com/ISE-SMILE/corral/api"

	log "github.com/sirupsen/logrus"
)

type DuplicationBackoffPolling struct {
	backoffCounter map[string]int
	PollLogger
}

func (b *DuplicationBackoffPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	predictionStartTime := time.Now().UnixNano()
	var backoff int

	b.PrematurePollMutex.Lock()
	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}
	b.PrematurePollMutex.Unlock()

	b.BackoffCounterMutex.Lock()
	if b.backoffCounter == nil {
		b.backoffCounter = make(map[string]int)
	}

	if last, ok := b.backoffCounter[RId]; ok {
		b.backoffCounter[RId] = last + last
		backoff = last
	} else {
		backoff = 4
		b.backoffCounter[RId] = backoff + backoff
	}
	b.BackoffCounterMutex.Unlock()

	predictionEndTime := time.Now().UnixNano()
	b.PollPredictionTimeMutex.Lock()
	if _, ok := b.PollPredictionTimes[RId]; ok {
		b.PollPredictionTimes[RId] += (predictionEndTime - predictionStartTime)
	} else {
		b.PollPredictionTimes[RId] = (predictionEndTime - predictionStartTime)
	}
	b.PollPredictionTimeMutex.Unlock()
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

func (b *DuplicationBackoffPolling) TaskUpdate(info api.TaskInfo) error {
	if info.Failed || info.Completed {
		b.BackoffCounterMutex.Lock()
		delete(b.backoffCounter, info.RId)
		b.BackoffCounterMutex.Unlock()
		return b.PollLogger.TaskUpdate(info)
	} else {
		return b.PollLogger.TaskUpdate(info)
	}
}
