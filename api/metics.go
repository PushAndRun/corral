package api

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var metricCollector *Metrics
var counter *Counter = &Counter{Counter: map[string]time.Duration{}}

func TryCollect(result map[string]interface{}) {
	if metricCollector != nil {
		metricCollector.Collect(result)
	}
}

func TryCount(key string, val time.Duration) {
	if counter != nil {
		counter.Count(key, val)
	}
}

func TryGetCount(key string) int {
	if counter != nil {
		return counter.GetAndReset(key)
	}
	return 0
}

type Counter struct {
	Counter map[string]time.Duration
	sync.Mutex
}

func (j *Counter) Count(key string, val time.Duration) {
	j.Mutex.Lock()
	if _, ok := j.Counter[key]; !ok {
		j.Counter[key] = val
	} else {
		j.Counter[key] += val
	}
	j.Mutex.Unlock()
}

func (j *Counter) GetAndReset(key string) int {
	j.Mutex.Lock()
	v := j.Counter[key]
	j.Counter[key] = 0
	j.Mutex.Unlock()
	return int(v.Round(time.Millisecond))
}

type Metrics struct {
	Fields map[string]string

	activationLog chan map[string]interface{}
	sem           chan struct{}
	open          bool
}

func (j *Metrics) AddField(key string, description string) error {
	if j.open {
		return fmt.Errorf("Metrics already open, can't add new fields if we already opend a csv file")
	}
	j.Fields[key] = description
	return nil
}

func (j *Metrics) Collect(result map[string]interface{}) {
	//inject counter valaues
	if j.activationLog != nil {
		j.activationLog <- result
	}

}

func (j *Metrics) writeActivationLog() {

	logName := fmt.Sprintf("%s_%s.csv",
		viper.GetString("logName"),
		time.Now().Format("2006_01_02"))

	if viper.IsSet("logDir") {
		logName = filepath.Join(viper.GetString("logDir"), logName)
	} else if dir := os.Getenv("CORRAL_LOGDIR"); dir != "" {
		logName = filepath.Join(dir, logName)
	}

	log.Info("Writing activation log to ", logName)

	writeHeder := checkHeader(logName, j.Fields)

	logFile, err := os.OpenFile(logName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Errorf("failed to open activation log @ %s - %f", logName, err)
		return
	}
	writer := bufio.NewWriter(logFile)
	logWriter := csv.NewWriter(writer)

	i := 0
	keys := make([]string, len(j.Fields))
	for k := range j.Fields {
		keys[i] = k
		i++
	}
	//ensure stable order
	sort.Strings(keys)

	if writeHeder != nil {
		log.Debugf("Writing header, because %s", writeHeder.Error())
		//write header

		err = logWriter.Write(keys)
		if err != nil {
			log.Errorf("failed to open activation log @ %s - %f", logName, err)
			return
		}
	}

	j.sem <- struct{}{}
	j.open = true
	for task := range j.activationLog {

		values := make([]string, len(keys))
		for i, k := range keys {
			if v, ok := task[k]; ok && (v != nil) {
				values[i] = fmt.Sprintf("%+v", v)
			}
		}
		err = logWriter.Write(values)
		if err != nil {
			log.Debugf("failed to write %+v - %f", task, err)
		}
		logWriter.Flush()
	}
	err = logFile.Close()
	log.Info("written metrics")
}

//checkHeader checks if the log file exists and if it does, it checks if the header in the file matches the fields, if error is not nil, the header is missing or invalid
func checkHeader(logName string, fields map[string]string) error {

	f, err := os.OpenFile(logName, os.O_RDONLY, 0666)
	if err == nil {
		defer f.Close()
		header, err := csv.NewReader(f).Read()
		if err != nil {
			return err
		}

		if len(header) != len(fields) {
			return fmt.Errorf("header length does not match fields length")
		}

		//check if present fields match what we expect
		i := 0
		for _, v := range header {
			if _, ok := fields[v]; !ok {
				return fmt.Errorf("header contains invalid field %s", v)
			}
			i++
		}
		if i != len(fields) {
			return fmt.Errorf("exsiting header is missing fields")
		}

		return nil
	}
	return err
}

func (j *Metrics) Start() {
	go j.writeActivationLog()
}

func (j *Metrics) Reset() {
	if j.activationLog != nil {
		close(j.activationLog)
	}
	<-j.sem
	j.open = false
	j.activationLog = make(chan map[string]interface{})
}

func (j *Metrics) Info() string {
	b := strings.Builder{}
	for k, v := range j.Fields {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	return b.String()
}

//CollectMetrics creates or gets the Metrics Singleton, and starts the activation log writer. Provided fields will be added to the log.
func CollectMetrics(fields map[string]string) (*Metrics, error) {

	if metricCollector == nil {
		m := Metrics{}
		m.Fields = make(map[string]string)
		m.activationLog = make(chan map[string]interface{})
		m.sem = make(chan struct{}, 1)
		metricCollector = &m
	}

	if fields != nil {
		for k, v := range fields {
			err := metricCollector.AddField(k, v)
			if err != nil {
				return nil, err
			}
		}
	}

	return metricCollector, nil
}
