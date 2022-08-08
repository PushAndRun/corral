package polling

import (
	"context"
	"time"

	"github.com/ISE-SMILE/corral/api"

	log "github.com/sirupsen/logrus"
)

type ExponentialBackoffPolling struct {
	backoffCounter map[string]int
	PollLogger
}

func (b *ExponentialBackoffPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	var backoff int

	if b.backoffCounter == nil {
		b.backoffCounter = make(map[string]int)
	}

	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}

	if last, ok := b.backoffCounter[RId]; ok {
		b.backoffCounter[RId] = last * last
		backoff = last
	} else {
		backoff = 2
		b.backoffCounter[RId] = backoff
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

func (b *ExponentialBackoffPolling) TaskUpdate(info api.TaskInfo) error {
	if info.Failed || info.Completed {
		delete(b.backoffCounter, info.RId)
		return b.PollLogger.TaskUpdate(info)
	} else {
		return b.PollLogger.TaskUpdate(info)
	}
}
