package polling

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/ISE-SMILE/corral/pkg/polling"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DNNModelPolling struct {
	PollLogger

	PolledTasks    map[string]bool
	backoffCounter map[string]int
}

var serverAddr = flag.String("addr", "localhost:8500", "The server address in the format of host:port")

func (b *DNNModelPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
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
		//we dont want to use the model again... fall back to dup.-backoff
		b.BackoffCounterMutex.Lock()
		if b.backoffCounter == nil {
			b.backoffCounter = make(map[string]int)
		}

		if last, ok := b.backoffCounter[RId]; ok {
			b.backoffCounter[RId] = last + last
			backoff = last
		} else {
			backoff = 16
			b.backoffCounter[RId] = backoff + backoff
		}
		b.BackoffCounterMutex.Unlock()
		log.Println("Use the fallback")

	} else {

		//get the prediction

		var sum int
		for _, val := range b.ExecutionTimes {
			sum += val
		}
		data := []uint64{5.0, 3109232.0, 2.0, 134217728.0, 134217728.0, 134217728.0, 32.0, 1.0, 134217728.0, 1.0, 0.0}
		backoff = b.GetPrediction(data) + timebuffer

		b.PolledTasks[RId] = true
		b.PolledTaskMutex.Unlock()
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

func (b *DNNModelPolling) GetPrediction(feat []uint64) int {

	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := polling.NewPollingDNNClient(conn)
	label, err := client.Predict(context.Background(), &polling.Features{Instances: feat})
	if err != nil {
		log.Fatalf("Unable to get the label: %v", err)
	}
	fmt.Sprintln("Received prediction: ", label.Prediction[0])
	return int(label.Prediction[0])
}
