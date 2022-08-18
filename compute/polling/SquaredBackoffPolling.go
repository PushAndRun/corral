package polling

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

type SquaredBackoffPolling struct {
	PollLogger
}

func (b *SquaredBackoffPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	predictionStartTime := time.Now().UnixNano()
	var backoff int
	offset := 10
	slope := 4

	b.PrematurePollMutex.Lock()
	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}
	b.PrematurePollMutex.Unlock()

	backoff = slope*(b.NumberOfPrematurePolls[RId]*b.NumberOfPrematurePolls[RId]) + offset

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
