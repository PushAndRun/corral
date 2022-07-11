package polling

import (
	"context"
	"github.com/ISE-SMILE/corral/api"
	log "github.com/sirupsen/logrus"
	"time"
)

type BackoffPolling struct {
	backoffCounter map[string]int
}

func (b *BackoffPolling) StartJob(info api.JobInfo) error {
	log.Debug(info)
	b.backoffCounter = make(map[string]int)
	return nil
}

func (b *BackoffPolling) TaskUpdate(info api.TaskInfo) error {
	log.Debug(info)
	if info.Completed || info.Failed {
		if _, ok := b.backoffCounter[info.RId]; ok {
			delete(b.backoffCounter, info.RId)
		}
	}
	return nil
}

func (b *BackoffPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	var backoff int
	if last, ok := b.backoffCounter[RId]; ok {
		b.backoffCounter[RId] = last + 1
		backoff = last
	} else {
		backoff = 4
		b.backoffCounter[RId] = backoff
	}
	log.Debugf("Poll backoff %s for %d seconds", RId, backoff*backoff)
	channel := make(chan interface{})
	go func() {
		select {
		case <-context.Done():
			channel <- struct{}{}
		case <-time.After(time.Second * time.Duration(backoff*backoff)):
			channel <- struct{}{}
		}
	}()
	return channel, nil
}
