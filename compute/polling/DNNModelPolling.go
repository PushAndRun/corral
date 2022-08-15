package polling

import (
	"context"
	"flag"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	address = "localhost:5000v1/models/pollingDNN:predict"
)

type DNNModelPolling struct {
	PollLogger

	PolledTasks    map[string]bool
	backoffCounter map[string]int
}

var (
	tls                = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	caFile             = flag.String("ca_file", "", "The file containing the CA root cert file")
	serverAddr         = flag.String("addr", "localhost:8500", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "x.test.example.com", "The server name used to verify the hostname returned by the TLS handshake")
)

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

		//get the average

		var sum int
		for _, val := range b.ExecutionTimes {
			sum += val
		}
		backoff = int(sum/len(b.ExecutionTimes)) + timebuffer
		log.Println("Use the average")
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

func (b *DNNModelPolling) GetPrediction(Features []uint64) int {
	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := pb.NewRouteGuideClient(conn)
}
