package corral

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"
)

type localExecutor struct {
	Start time.Time
}

func (l *localExecutor) RunMapper(job *Job, jobNumber int, binID uint, inputSplits []InputSplit) error {
	estart := time.Now()
	// Precaution to avoid running out of memory for reused Lambdas
	debug.FreeOSMemory()

	err := job.runMapper(binID, inputSplits)

	eend := time.Now()
	result := taskResult{
		BytesRead:    int(job.bytesRead),
		BytesWritten: int(job.bytesWritten),
		HId:          "local",
		CId:          "local",
		JId:          fmt.Sprintf("%d_%d_%d", jobNumber, 0, binID),
		RId:          strconv.Itoa(jobNumber),
		CStart:       l.Start.Unix(),
		EStart:       estart.Unix(),
		EEnd:         eend.Unix(),
	}

	job.Collect(result)
	return err
}

func (l *localExecutor) RunReducer(job *Job, jobNumber int, binID uint) error {
	estart := time.Now()
	// Precaution to avoid running out of memory for reused Lambdas
	debug.FreeOSMemory()

	err := job.runReducer(binID)

	eend := time.Now()
	result := taskResult{
		BytesRead:    int(job.bytesRead),
		BytesWritten: int(job.bytesWritten),
		HId:          "local",
		CId:          "local",
		JId:          fmt.Sprintf("%d_%d_%d", jobNumber, 1, binID),
		RId:          strconv.Itoa(jobNumber),
		CStart:       l.Start.Unix(),
		EStart:       estart.Unix(),
		EEnd:         eend.Unix(),
	}

	job.Collect(result)
	return err
}

func (l *localExecutor) HintSplits(splits uint) error {
	runtime.GOMAXPROCS(int(splits << 2))
	return nil
}
