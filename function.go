package corral

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/internal/corcache"
	"github.com/ISE-SMILE/corral/internal/corfs"

	log "github.com/sirupsen/logrus"
)

//takes a driver and a hostID and requestID and returns the main worker function for these parameters
func handle(driver *Driver, hostID func() string, requestID func() string) func(task api.Task) (api.TaskResult, error) {
	return func(task api.Task) (api.TaskResult, error) {
		estart := time.Now()
		// Precaution to avoid running out of memory for reused Lambdas
		debug.FreeOSMemory()
		result := api.TaskResult{
			HId:    hostID(),
			CId:    driver.runtimeID,
			RId:    requestID(),
			JId:    fmt.Sprintf("%d_%d_%d", task.JobNumber, task.Phase, task.BinID),
			CStart: driver.Start.UnixNano(),
			EStart: estart.UnixNano(),
		}
		// Setup current job
		//we can assume that this dose not change all the time
		var err error
		fs, err := corfs.InitFilesystem(task.FileSystemType)
		if err != nil {
			result.EEnd = time.Now().UnixNano()
			return result, fmt.Errorf("fs init error +%v", err)
		}
		log.Debugf("%d - %+v", task.FileSystemType, task)
		currentJob := driver.jobs[task.JobNumber]

		if task.CacheSystemType != api.NoCache {
			cache, err := corcache.NewCacheSystem(task.CacheSystemType)
			if err != nil {
				result.EEnd = time.Now().UnixNano()
				return result, fmt.Errorf("cache create error +%v", err)
			}
			//need to init the cache before use..
			err = cache.Init()
			if err != nil {
				result.EEnd = time.Now().UnixNano()
				return result, fmt.Errorf("cache init error +%v", err)
			}

			currentJob.cacheSystem = cache
		}

		driver.currentJob = task.JobNumber
		currentJob.fileSystem = fs
		currentJob.intermediateBins = task.IntermediateBins
		currentJob.outputPath = task.WorkingLocation
		currentJob.config.Cleanup = task.Cleanup

		// Need to reset job counters in case this is a reused lambda
		currentJob.bytesRead = 0
		currentJob.bytesWritten = 0

		switch task.Phase {
		case api.MapPhase:
			err = currentJob.runMapper(task.BinID, task.Splits)
		case api.ReducePhase:
			err = currentJob.runReducer(task.BinID)
		default:
			err = fmt.Errorf("Unknown phase: %d", task.Phase)
		}

		result.BytesRead = int(currentJob.bytesRead)
		result.BytesWritten = int(currentJob.bytesWritten)
		result.EEnd = time.Now().UnixNano()

		result.SWT = int64(api.TryGetCount("SWT"))
		result.SRT = int64(api.TryGetCount("SRT"))
		result.CWT = int64(api.TryGetCount("CWT"))
		result.CRT = int64(api.TryGetCount("CRT"))

		return result, err
	}
}
