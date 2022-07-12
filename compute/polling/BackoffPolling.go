package polling

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"time"

	struct2csv "github.com/dnlo/struct2csv"

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
	backoffCounter map[string]int

	//Maps to hold the task and job infos for measuring the polling performance
	taskInfos map[string]api.TaskInfo
	jobInfos  map[string]api.JobInfo
}

func (b *BackoffPolling) StartJob(info api.JobInfo) error {
	log.Debug(info)
	b.backoffCounter = make(map[string]int)

	if b.taskInfos == nil {
		b.taskInfos = make(map[string]api.TaskInfo)
	}
	if b.jobInfos == nil {
		b.jobInfos = make(map[string]api.JobInfo)
	}

	b.jobInfos[strconv.Itoa(info.JobId)] = info

	return nil
}

func (b *BackoffPolling) TaskUpdate(info api.TaskInfo) error {
	log.Info("TaskUpdate called with: " + fmt.Sprint(info))

	taskId := strconv.Itoa(info.JobId) + "-" + strconv.Itoa(info.TaskId)

	if val, ok := b.taskInfos[taskId]; ok {
		//update entry
		mergo.Merge(&val, info, mergo.WithTransformers(timeTransformer{}))
		b.taskInfos[taskId] = val
	} else {
		//create entry
		b.taskInfos[taskId] = info
	}

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
