package polling

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/imdario/mergo"

	struct2csv "github.com/dnlo/struct2csv"

	"github.com/ISE-SMILE/corral/api"

	log "github.com/sirupsen/logrus"
)

type PollLogger struct {
	//Maps to hold the task and job infos for measuring the polling performance
	taskInfos   map[string]api.TaskInfo
	jobInfos    map[string]api.JobInfo
	rIdToJobId  map[string]string
	rIdToTaskId map[string]string

	NumberOfPrematurePolls map[string]int
	FinalPollTime          map[string]int64
	PollPredictionTimes    map[string]int64
	ExecutionTimes         []int

	PollingLabel            string
	TaskMapMutex            sync.RWMutex
	JobMapMutex             sync.RWMutex
	PrematurePollMutex      sync.RWMutex
	PollTimeMutex           sync.RWMutex
	PolledTaskMutex         sync.RWMutex
	PollPredictionTimeMutex sync.RWMutex
	BackoffCounterMutex     sync.RWMutex
	IdMapMutex              sync.RWMutex
}

func (b *PollLogger) StartJob(info api.JobInfo) error {
	log.Debug(info)
	b.NumberOfPrematurePolls = make(map[string]int)
	b.FinalPollTime = make(map[string]int64)
	b.PollPredictionTimes = make(map[string]int64)
	b.ExecutionTimes = make([]int, 1)
	b.ExecutionTimes[0] = 16

	b.TaskMapMutex = sync.RWMutex{}
	b.PollTimeMutex = sync.RWMutex{}
	b.PrematurePollMutex = sync.RWMutex{}
	b.PolledTaskMutex = sync.RWMutex{}
	b.PollPredictionTimeMutex = sync.RWMutex{}
	b.BackoffCounterMutex = sync.RWMutex{}
	b.JobMapMutex = sync.RWMutex{}

	if b.taskInfos == nil {
		b.taskInfos = make(map[string]api.TaskInfo)
	}
	if b.jobInfos == nil {
		b.jobInfos = make(map[string]api.JobInfo)
	}
	if b.rIdToJobId == nil {
		b.rIdToJobId = make(map[string]string)
	}
	if b.rIdToTaskId == nil {
		b.rIdToTaskId = make(map[string]string)
	}

	b.JobMapMutex.Lock()
	b.jobInfos[fmt.Sprint(info.JobId)] = info
	b.JobMapMutex.Unlock()

	return nil
}

func (b *PollLogger) JobUpdate(info api.JobInfo) error {

	b.JobMapMutex.Lock()
	if val, ok := b.jobInfos[fmt.Sprint(info.JobId)]; ok {
		//update entry
		mergo.MergeWithOverwrite(&val, info, mergo.WithTransformers(&timeTransformer{}))
		b.jobInfos[fmt.Sprint(info.JobId)] = val
	} else {
		//create entry
		b.jobInfos[fmt.Sprint(info.JobId)] = info
	}
	b.JobMapMutex.Unlock()
	return nil
}

func (b *PollLogger) TaskUpdate(info api.TaskInfo) error {
	log.Info("TaskUpdate called with: " + fmt.Sprint(info))

	b.IdMapMutex.Lock()
	if _, ok := b.rIdToJobId[info.RId]; !ok {
		b.rIdToJobId[info.RId] = info.JobId
	}
	if _, ok := b.rIdToTaskId[info.RId]; !ok {
		b.rIdToTaskId[info.RId] = info.TaskId
	}
	b.IdMapMutex.Unlock()

	b.TaskMapMutex.Lock()
	if val, ok := b.taskInfos[fmt.Sprint(info.TaskId)]; ok {
		//update entry
		mergo.MergeWithOverwrite(&val, info, mergo.WithTransformers(&timeTransformer{}))
		b.taskInfos[fmt.Sprint(info.TaskId)] = val
	} else {
		//create entry
		b.taskInfos[fmt.Sprint(info.TaskId)] = info
	}
	b.TaskMapMutex.Unlock()

	if info.Completed || info.Failed {
		b.TaskMapMutex.Lock()

		val := b.taskInfos[info.TaskId]
		//merge new information
		b.PollTimeMutex.Lock()
		mergo.MergeWithOverwrite(&val, api.TaskInfo{
			NumberOfPrematurePolls: b.NumberOfPrematurePolls[info.RId],
			FinalPollTime:          b.FinalPollTime[info.RId],
			RId:                    info.RId,
			PollCalculationTime:    b.PollPredictionTimes[info.RId],
		}, mergo.WithTransformers(&timeTransformer{}))
		b.taskInfos[info.TaskId] = val
		b.PollTimeMutex.Unlock()

		//compute additional statistics
		if entry, ok := b.taskInfos[info.TaskId]; ok {
			b.PollTimeMutex.Lock()
			if int64(time.Nanosecond*time.Duration(b.FinalPollTime[info.RId])-time.Nanosecond*time.Duration(info.FunctionExecutionEnd)) > 0 {
				entry.PollLatency = int64(time.Nanosecond*time.Duration(b.FinalPollTime[info.RId]) - time.Nanosecond*time.Duration(info.FunctionExecutionEnd))
				entry.TotalExecutionTime = entry.RequestCompletedAndPolled.UnixNano() - entry.RequestStart.UnixNano() - entry.PollLatency
			} else {
				entry.TotalExecutionTime = entry.RequestCompletedAndPolled.UnixNano() - entry.RequestStart.UnixNano()
			}
			b.PollTimeMutex.Unlock()
			entry.FunctionStartLatency = entry.FunctionExecutionStart - entry.RequestStart.UnixNano()

			if entry.Phase == 0 {
				job := b.jobInfos[entry.JobId]
				if _, ok := job.MapBinSizes[entry.BinId]; ok {
					entry.BinSize = job.MapBinSizes[entry.BinId]
				}
			}
			if entry.Phase == 1 {
				job := b.jobInfos[entry.JobId]
				if _, ok := job.ReduceBinSizes[entry.BinId]; ok {
					entry.BinSize = job.ReduceBinSizes[entry.BinId]
				}
			}

			executionTimeInSeconds := int(entry.TotalExecutionTime / 1000000000)
			if executionTimeInSeconds > 0 && executionTimeInSeconds < 500 {
				b.ExecutionTimes = append(b.ExecutionTimes, executionTimeInSeconds)
			}

			b.taskInfos[info.TaskId] = entry

		}

		b.TaskMapMutex.Unlock()
		b.PrematurePollMutex.Lock()
		delete(b.NumberOfPrematurePolls, info.RId)
		b.PrematurePollMutex.Unlock()
		b.PollTimeMutex.Lock()
		delete(b.FinalPollTime, info.RId)
		b.PollTimeMutex.Unlock()
	}

	return nil
}

func (b *PollLogger) SetFinalPollTime(RId string, timeNano int64) {
	b.PollTimeMutex.Lock()
	b.FinalPollTime[RId] = timeNano
	b.PollTimeMutex.Unlock()
}

func (b *PollLogger) Finalize() error {

	err := b.jobLogWriter("jobLog", b.jobInfos)
	if err != nil {
		return err
	}

	err = b.taskLogWriter("taskLog", b.taskInfos)
	return err
}

func (b *PollLogger) jobLogWriter(logName string, logStruct map[string]api.JobInfo) error {

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

func (b *PollLogger) taskLogWriter(logName string, logStruct map[string]api.TaskInfo) error {

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
