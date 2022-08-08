package polling

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

type AveragePolling struct {
	PollLogger
	backoffCounter map[string]int
}

func (b *AveragePolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	var backoff int

	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}

	//poll logic
	if last, ok := b.backoffCounter[RId]; ok {
		b.backoffCounter[RId] = last + last
		backoff = last + last
	} else {
		backoff = 256
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
