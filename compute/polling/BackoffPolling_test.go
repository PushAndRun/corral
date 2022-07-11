package polling

import (
	"context"
	"github.com/ISE-SMILE/corral/api"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestBackoffPolling(t *testing.T) {
	ctx := context.Background()
	polling := &BackoffPolling{}
	polling.StartJob(api.JobInfo{
		JobId:          1,
		Splits:         10,
		SplitSize:      512,
		MapBinSize:     512,
		ReduceBinSize:  512,
		MaxConcurrency: 2,
		Backend:        "local",
		FunctionMemory: 512,
		CacheType:      0,
		MapLOC:         0,
		ReduceLoc:      0,
	})

	polling.TaskUpdate(api.TaskInfo{
		JobId:          1,
		TaskId:         1,
		Phase:          0,
		RequestStart:   time.Now(),
		NumberOfInputs: 10,
	})

	polling.TaskUpdate(api.TaskInfo{
		RId:    "foo",
		JobId:  1,
		TaskId: 1,
		Phase:  0,
	})

	_, err := polling.Poll(ctx, "foo")
	assert.Nil(t, err, "expected p to be nill")
	polling.TaskUpdate(api.TaskInfo{
		RId:           "foo",
		NumberOfPolls: 1,
	})
	_, err = polling.Poll(ctx, "foo")
	assert.Nil(t, err, "expected p to be nill")

	polling.TaskUpdate(api.TaskInfo{
		JobId:     1,
		TaskId:    1,
		Phase:     0,
		Completed: true,
	})

	_, err = polling.Poll(ctx, "foo")
	//TODO: up to u to decided what happens to already completed tasks

	_, err = polling.Poll(ctx, "bar")
	assert.Nil(t, err, "expected p to be nill")
	//this should not fail

	err = polling.StartJob(api.JobInfo{
		JobId:          1,
		Splits:         10,
		SplitSize:      512,
		MapBinSize:     512,
		ReduceBinSize:  512,
		MaxConcurrency: 2,
		Backend:        "local",
		FunctionMemory: 512,
		CacheType:      0,
		MapLOC:         0,
		ReduceLoc:      0,
	})
	assert.Nil(t, err, "expected p to be nil")
}
