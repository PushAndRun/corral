package polling

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

type AveragePolling struct {
	PollLogger
	ExecutionTimes []int64
	PolledTasks    map[string]bool
	backoffCounter map[string]int
}

func (b *AveragePolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	var backoff int
	timebuffer := 2

	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}

	if b.PolledTasks == nil {
		b.PolledTasks = make(map[string]bool)
	}

	if _, ok := b.PolledTasks[RId]; !ok {
		if b.ExecutionTimes == nil {
			b.ExecutionTimes = make([]int64, 1)
			b.ExecutionTimes[0] = 4
		} else {
			var sum int64
			for _, val := range b.ExecutionTimes {
				sum += val
			}
			sum = sum / 1000000000
			backoff = (int(sum) / len(b.ExecutionTimes)) + timebuffer
			b.PolledTasks[RId] = true
		}
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

func (b *AveragePolling) SetFinalPollTime(RId string, timeNano int64) {
	b.PollLogger.SetFinalPollTime(RId, timeNano)

	var startTime int64
	for _, val := range b.taskInfos {
		if val.RId == RId {
			startTime = val.RequestStart.UnixNano()
		}
	}
	b.ExecutionTimes = append(b.ExecutionTimes, timeNano-startTime)
}
