package api

import (
	"context"
	"time"
)

type JobInfo struct {
	JobId int
	//Number of the Query to associate the records with the experiment
	QueryID int
	//Polling algorithm that was used
	PollingStrategy string
	//Total number of Inputs for this job
	Splits int
	//SplitSize of this job in byte
	SplitSize int64
	//the maximum number of bytes per pin in the map phase
	MapBinSize int64
	//Maximum input size for reduce function
	ReduceBinSize int64
	//Maximum number of allowed concurrent function calls
	MaxConcurrency int
	//Used Backend Type, e.g., whisk, local or lambda ...
	Backend string
	//Used function Memmory in Megabyte
	FunctionMemory int
	//CacheType reference
	CacheType int

	//estimated lines of code for the user defined map function
	MapLOC int
	//estimated lines of code for the user defined reduce function
	ReduceLoc int
}

type TaskInfo struct {
	RId string
	//unique job id
	JobId int
	//unique task id
	TaskId int
	//indecates map/reduce phase
	Phase int
	//time the task was sent to the backend
	RequestStart time.Time
	//time the task was successfully polled by the backend
	RequestReceived time.Time

	//Duration of the task Execution
	ExecutionDuration time.Duration
	//RuntimeId - semi unique identifier of the used execution runtime
	RuntimeId string

	//Number of Inputs for this Task
	NumberOfInputs int
	//Number of Polls for this Task
	NumberOfPolls int
	//Indicates if this task is completed, e.g., executed successfully
	Completed bool
	//Indicates if this task failed
	Failed bool
}

type PollingStrategy interface {
	/*StartJob initializes a Job, all subsequent TaskUpdates are treated
	  as related to this Job. Calling StartJob again indicates the start of a
	  new job and the end of the last job.
	*/
	StartJob(JobInfo) error

	/*
		TaskUpdate updates metadata related to a task. Usually called after a Polling
		attempt.
	*/
	TaskUpdate(TaskInfo) error

	/*
		Poll blocks until the given task should be polled.
		Poll returns a channel that returns once a poll should be performed.
		This channel should only fire once. To cancel a poll use the context.
	*/
	Poll(context context.Context, RId string) (<-chan interface{}, error)

	/*used to coordinate the log creation*/
	Finalize() error
}
