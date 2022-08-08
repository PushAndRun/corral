package polling

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

type ConstantPolling struct {
	PollLogger
}

func (b *ConstantPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	timer := 5

	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}

	log.Debugf("Poll %s again after %d seconds", RId, timer)
	channel := make(chan interface{})
	go func() {
		select {
		case <-context.Done():
			channel <- struct{}{}
		case <-time.After(time.Second * time.Duration(timer)):
			channel <- struct{}{}
		}
	}()
	return channel, nil
}
