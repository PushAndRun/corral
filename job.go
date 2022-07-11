package corral

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ISE-SMILE/corral/api"
	humanize "github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// Job is the logical container for a MapReduce job
type Job struct {
	Map           Mapper
	Reduce        Reducer
	PartitionFunc PartitionFunc

	PauseFunc PauseFunc
	StopFunc  StopFunc
	HintFunc  HintFunc

	fileSystem  api.FileSystem
	cacheSystem api.CacheSystem

	config           *config
	intermediateBins uint
	outputPath       string

	bytesRead    int64
	bytesWritten int64
	metrics      *api.Metrics
}

//type SortFunc func() int

type SortJob struct {
	Job
}

// Logic for running a single map Task
func (j *Job) runMapper(mapperID uint, splits []api.InputSplit) error {
	//check if we can use a cacheFS instead
	var fs api.FileSystem = j.fileSystem
	if j.cacheSystem != nil {
		fs = j.cacheSystem
	}

	emitter := newMapperEmitter(j.intermediateBins, mapperID, j.outputPath, fs)
	if j.PartitionFunc != nil {
		emitter.partitionFunc = j.PartitionFunc
	}

	for _, split := range splits {
		err := j.runMapperSplit(split, &emitter)
		if err != nil {
			return fmt.Errorf("split error +%v", err)
		}
	}

	atomic.AddInt64(&j.bytesWritten, emitter.bytesWritten())
	return emitter.close()
}

func splitInputRecord(record string) *keyValue {
	//XXX: in case of map this will just cost a lot of unnassary compute...
	fields := strings.Split(record, "\t")
	if len(fields) == 2 {
		return &keyValue{
			Key:   fields[0],
			Value: fields[1],
		}
	}
	return &keyValue{
		Value: record,
	}
}

// runMapperSplit runs the mapper on a single InputSplit
func (j *Job) runMapperSplit(split api.InputSplit, emitter Emitter) error {
	offset := split.StartOffset
	if split.StartOffset != 0 {
		offset--
	}

	inputSource, err := j.fileSystem.OpenReader(split.Filename, split.StartOffset)
	if err != nil {
		return fmt.Errorf("read error +%v", err)
	}

	scanner := bufio.NewScanner(inputSource)
	var bytesRead int64
	splitter := countingSplitFunc(bufio.ScanLines, &bytesRead)
	scanner.Split(splitter)

	if split.StartOffset != 0 {
		scanner.Scan()
	}

	for scanner.Scan() {
		record := scanner.Text()
		kv := splitInputRecord(record)
		//inject the filename in case we have no other key...
		if kv.Key == "" {
			kv.Key = split.Filename
		}
		j.Map.Map(kv.Key, kv.Value, emitter)

		// Stop reading when end of InputSplit is reached
		pos := bytesRead
		if split.Size() > 0 && pos > split.Size() {
			break
		}
	}

	atomic.AddInt64(&j.bytesRead, bytesRead)

	return nil
}

// Logic for running a single reduce Task
func (j *Job) runReducer(binID uint) error {
	//check if we can use a cacheFS instead
	var fs api.FileSystem = j.fileSystem
	if j.cacheSystem != nil {
		fs = j.cacheSystem
	}

	// Determine the intermediate data files this reducer is responsible for
	path := fs.Join(j.outputPath, fmt.Sprintf("map-bin%d-*", binID))
	files, err := fs.ListFiles(path)
	if err != nil {
		return err
	}

	// Open emitter for output data
	path = j.fileSystem.Join(j.outputPath, fmt.Sprintf("output-part-%d", binID))
	emitWriter, err := j.fileSystem.OpenWriter(path)

	if err != nil {
		return err
	}
	defer emitWriter.Close()

	data := make(map[string][]string, 0)
	var bytesRead int64

	for _, file := range files {
		reader, err := fs.OpenReader(file.Name, 0)
		bytesRead += file.Size
		if err != nil {
			return err
		}

		// Feed intermediate data into reducers
		decoder := json.NewDecoder(reader)
		for decoder.More() {
			var kv keyValue
			if err := decoder.Decode(&kv); err != nil {
				return err
			}

			if _, ok := data[kv.Key]; !ok {
				data[kv.Key] = make([]string, 0)
			}

			data[kv.Key] = append(data[kv.Key], kv.Value)
		}
		reader.Close()

		// Delete intermediate map data
		if j.config.Cleanup {
			err := fs.Delete(file.Name)
			if err != nil {
				log.Error(err)
			}
		}
	}

	var waitGroup sync.WaitGroup
	sem := semaphore.NewWeighted(10)

	emitter := newReducerEmitter(emitWriter)
	for key, values := range data {
		sem.Acquire(context.Background(), 1)
		waitGroup.Add(1)
		go func(key string, values []string) {
			defer sem.Release(1)

			keyChan := make(chan string)
			keyIter := newValueIterator(keyChan)

			go func() {
				defer waitGroup.Done()
				j.Reduce.Reduce(key, keyIter, emitter)
			}()

			for _, value := range values {
				// Pass current value to the appropriate key channel
				keyChan <- value
			}
			close(keyChan)
		}(key, values)
	}

	waitGroup.Wait()

	atomic.AddInt64(&j.bytesWritten, emitter.bytesWritten())
	atomic.AddInt64(&j.bytesRead, bytesRead)

	return nil
}

// inputSplits calculates all input files' inputSplits.
// inputSplits also determines and saves the number of intermediate bins that will be used during the shuffle.
func (j *Job) inputSplits(inputs []string, maxSplitSize int64) []api.InputSplit {
	files := make([]string, 0)
	for _, inputPath := range inputs {
		fileInfos, err := j.fileSystem.ListFiles(inputPath)
		if err != nil {
			log.Warn(err)
			continue
		}

		for _, fInfo := range fileInfos {
			files = append(files, fInfo.Name)
		}
	}

	splits := make([]api.InputSplit, 0)
	var totalSize int64
	for _, inputFileName := range files {
		fInfo, err := j.fileSystem.Stat(inputFileName)
		if err != nil {
			log.Warnf("Unable to load input file: %s (%s)", inputFileName, err)
			continue
		}

		totalSize += fInfo.Size
		splits = append(splits, splitInputFile(fInfo, maxSplitSize)...)
	}
	if len(files) > 0 && len(splits) > 0 {
		log.Debugf("Average split size: %s bytes", humanize.Bytes(uint64(totalSize)/uint64(len(splits))))
	}

	j.intermediateBins = uint(float64(totalSize/j.config.ReduceBinSize) * 1.25)
	if j.intermediateBins == 0 {
		j.intermediateBins = 1
	}

	return splits
}

func (j *Job) done() {
	if j.metrics != nil {
		j.metrics.Reset()
	}
}

//needs to run in a process
func (j *Job) CollectMetrics() {
	metrics, err := api.CollectMetrics(map[string]string{
		"RId":          "request identifier",
		"CId":          "container identifier",
		"HId":          "vm id",
		"JId":          "job id",
		"cStart":       "container start time",
		"eStart":       "execution start",
		"eEnd":         "execution end",
		"BytesRead":    "bytes read",
		"BytesWritten": "bytes written",
		"ObjectWrite":  "ms spent writing to object store",
		"ObjectRead":   "ms spent reading from object store",
		"CacheWrite":   "ms spent writing to cache",
		"CacheRead":    "ms spent reading from cache",
	})

	if err != nil {
		log.Errorf("could not collect metrics. cause: %+v", err)
	} else {
		j.metrics = metrics
		j.metrics.Start()
	}
}

func (j *Job) Collect(result api.TaskResult) {
	if j.metrics != nil {
		j.metrics.Collect(map[string]interface{}{
			"RId":          result.RId,
			"CId":          result.CId,
			"HId":          result.HId,
			"JId":          result.JId,
			"cStart":       result.CStart,
			"eStart":       result.EStart,
			"eEnd":         result.EEnd,
			"BytesRead":    result.BytesRead,
			"BytesWritten": result.BytesWritten,
			"ObjectWrite":  result.SWT,
			"ObjectRead":   result.SRT,
			"CacheWrite":   result.CWT,
			"CacheRead":    result.CRT,
		})
	}
}

// NewJob creates a new job from a Mapper and Reducer.
func NewJob(mapper Mapper, reducer Reducer) *Job {
	job := &Job{
		Map:    mapper,
		Reduce: reducer,
		config: &config{},
	}

	return job
}
