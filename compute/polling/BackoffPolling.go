package polling

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	struct2csv "github.com/dnlo/struct2csv"

	"time"

	"github.com/ISE-SMILE/corral/api"
	"github.com/imdario/mergo"
	log "github.com/sirupsen/logrus"
)

//Transformer is an addition from https://pkg.go.dev/github.com/imdario/mergo@v0.3.13 and supplements mergo with a handling for zero time stamps
type timeTransformer struct {
}

func (t timeTransformer) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	if typ == reflect.TypeOf(time.Time{}) {
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("IsZero")
				result := isZero.Call([]reflect.Value{})
				if result[0].Bool() {
					dst.Set(src)
				}
			}
			return nil
		}
	}
	return nil
}

type BackoffPolling struct {
	backoffCounter         map[string]int
	NumberOfPrematurePolls map[string]int
	FinalPollTime          map[string]int64
	//Maps to hold the task and job infos for measuring the polling performance
	taskInfos map[string]api.TaskInfo
	jobInfos  map[string]api.JobInfo
}

func (b *BackoffPolling) StartJob(info api.JobInfo) error {
	log.Debug(info)
	info.PollingStrategy = "BackoffPolling"
	b.backoffCounter = make(map[string]int)
	b.NumberOfPrematurePolls = make(map[string]int)
	b.FinalPollTime = make(map[string]int64)

	if b.taskInfos == nil {
		b.taskInfos = make(map[string]api.TaskInfo)
	}
	if b.jobInfos == nil {
		b.jobInfos = make(map[string]api.JobInfo)
	}

	b.jobInfos[fmt.Sprint(info.JobId)] = info

	return nil
}

func (b *BackoffPolling) JobUpdate(info api.JobInfo) error {
	if val, ok := b.jobInfos[fmt.Sprint(info.JobId)]; ok {
		//update entry
		mergo.MergeWithOverwrite(&val, info, mergo.WithTransformers(timeTransformer{}))
		b.jobInfos[fmt.Sprint(info.JobId)] = val
	} else {
		//create entry
		b.jobInfos[fmt.Sprint(info.JobId)] = info
	}
	return nil
}

func (b *BackoffPolling) UpdateJob(info api.JobInfo) error {
	log.Debug(info)
	return nil
}

func (b *BackoffPolling) TaskUpdate(info api.TaskInfo) error {
	log.Info("TaskUpdate called with: " + fmt.Sprint(info))

	if val, ok := b.taskInfos[fmt.Sprint(info.TaskId)]; ok {
		//update entry
		mergo.MergeWithOverwrite(&val, info, mergo.WithTransformers(timeTransformer{}))
		b.taskInfos[fmt.Sprint(info.TaskId)] = val
	} else {
		//create entry
		b.taskInfos[fmt.Sprint(info.TaskId)] = info
	}

	if info.Completed || info.Failed {
		if _, ok := b.NumberOfPrematurePolls[info.RId]; ok {
			val := b.taskInfos[info.TaskId]

			//merge new information
			mergo.MergeWithOverwrite(&val, api.TaskInfo{
				NumberOfPrematurePolls: b.NumberOfPrematurePolls[info.RId],
				FinalPollTime:          b.FinalPollTime[info.RId],
				RId:                    info.RId,
			}, mergo.WithTransformers(timeTransformer{}))
			b.taskInfos[info.TaskId] = val

			//compute additional statistics
			if entry, ok := b.taskInfos[info.TaskId]; ok {
				entry.PollLatency = int64(time.Nanosecond*time.Duration(b.FinalPollTime[info.RId]) - time.Nanosecond*time.Duration(info.FunctionExecutionEnd))
				entry.TotalExecutionTime = entry.RequestReceived.UnixNano() - entry.RequestStart.UnixNano()
				b.taskInfos[info.TaskId] = entry
			}

			delete(b.backoffCounter, info.RId)
			delete(b.NumberOfPrematurePolls, info.RId)
			delete(b.FinalPollTime, info.RId)
		}
	}
	return nil
}

func (b *BackoffPolling) SetFinalPollTime(RId string, timeNano int64) {
	b.FinalPollTime[RId] = timeNano
}

func (b *BackoffPolling) Poll(context context.Context, RId string) (<-chan interface{}, error) {
	var backoff int

	if polls, ok := b.NumberOfPrematurePolls[RId]; ok {
		b.NumberOfPrematurePolls[RId] = polls + 1
	} else {
		b.NumberOfPrematurePolls[RId] = 1
	}

	if last, ok := b.backoffCounter[RId]; ok {
		b.backoffCounter[RId] = last * 2
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

func (b *BackoffPolling) Finalize() error {

	err := b.jobLogWriter("jobLog", b.jobInfos)
	if err != nil {
		return err
	}

	err = b.taskLogWriter("taskLog", b.taskInfos)
	return err
}

func (b *BackoffPolling) jobLogWriter(logName string, logStruct map[string]api.JobInfo) error {

	logFile, err := os.OpenFile(logName+".csv", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)

	writer := bufio.NewWriter(logFile)
	reader := bufio.NewReader(logFile)
	logWriter := csv.NewWriter(writer)
	logReader := csv.NewReader(reader)
	enc := struct2csv.New()
	rows := [][]string{}

	defer logWriter.Flush()
	defer logFile.Close()

	if err != nil {
		log.Errorf("Failed to open "+logName+" @ %s - %f", logName, err)
		return err
	} else {
		_, err := logReader.Read()
		if err != nil && err != io.EOF {
			log.Error("Failed to read header for " + logName + " " + fmt.Sprint(err))
			return err
		}
		if err == io.EOF {

			keys := []string{}
			for k := range logStruct {
				keys = append(keys, k)
			}

			if len(keys) == 0 {
				log.Error("Empty LogStruct for " + logName)
				return nil
			}

			colhdrs, err := enc.GetColNames(logStruct[keys[0]])
			if err != nil {
				log.Error("Failed to write headers for " + logName)
				return err
			}
			rows = append(rows, colhdrs)
		}
	}

	for _, value := range logStruct {
		row, err := enc.GetRow(value)
		if err != nil {
			log.Error("Error when parsing job log to csv")
		}
		rows = append(rows, row)
	}

	logWriter.WriteAll(rows)
	return err
}

func (b *BackoffPolling) taskLogWriter(logName string, logStruct map[string]api.TaskInfo) error {

	logFile, err := os.OpenFile(logName+".csv", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)

	writer := bufio.NewWriter(logFile)
	reader := bufio.NewReader(logFile)
	logWriter := csv.NewWriter(writer)
	logReader := csv.NewReader(reader)
	enc := struct2csv.New()
	rows := [][]string{}

	defer logWriter.Flush()
	defer logFile.Close()

	if err != nil {
		log.Errorf("Failed to open "+logName+" @ %s - %f", logName, err)
		return err
	} else {
		_, err := logReader.Read()
		if err != nil && err != io.EOF {
			log.Error("Failed to read header for " + logName + " " + fmt.Sprint(err))
			return err
		}
		if err == io.EOF {

			keys := []string{}
			for k := range logStruct {
				keys = append(keys, k)
			}

			if len(keys) == 0 {
				log.Error("Empty LogStruct for " + logName)
				return nil
			}

			colhdrs, err := enc.GetColNames(logStruct[keys[0]])
			if err != nil {
				log.Error("Failed to write headers for " + logName)
				return err
			}
			rows = append(rows, colhdrs)
		}
	}

	for _, value := range logStruct {
		row, err := enc.GetRow(value)
		if err != nil {
			log.Error("Error when parsing task log to csv")
		}
		rows = append(rows, row)
	}

	logWriter.WriteAll(rows)
	return err
}
