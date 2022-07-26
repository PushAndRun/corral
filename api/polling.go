package api

import (
	"context"
	"time"
)

type JobInfo struct {
	JobId string
	//Number of the Query to associate the records with the experiment
	TPCHQueryID string
	//Polling algorithm that was used
	PollingStrategy string
	//Total number of jobs
	NumberOfJobs int
	//Current job number
	JobNumber int
	//Bytes written by the previous job
	PrevJobBytesWritten int64
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

	//estimated complexity for the user defined map function (1,2,3)
	MapComplexity ComplexityType
	//estimated complexity e for the user defined reduce function (1,2,3)
	ReduceComplexity ComplexityType

	//execution time of the job without polling latency
	ExecutionTime int64

	//Text field to hold additional information about the experiment
	ExperimentNote string

	MapBinSizes    map[int]int64
	ReduceBinSizes map[int]int64
}

type ComplexityType int32

const (
	EASY   ComplexityType = 1
	MEDIUM ComplexityType = 2
	HIGH   ComplexityType = 3
)

type TaskInfo struct {
	RId string
	//unique job id
	JobId string
	//unique task id
	TaskId string
	//RuntimeId - semi unique identifier of the used execution runtime
	RuntimeId string

	//indecates map/reduce phase
	Phase int
	//The number of the job
	JobNumber int
	//Number of Inputs for this task
	NumberOfInputs int
	//Id of the Bin
	BinId int
	//Size of the Bin
	BinSize int64

	//Total task execution duration including function execution duration and all latencies in ns
	TotalExecutionTime int64
	//Time span from request start to function start
	FunctionStartLatency int64
	//Duration of the function Execution
	FunctionExecutionDuration int64
	// Latency between task completion and final poll in ns
	PollLatency int64

	//Number of premature Polls for this task
	NumberOfPrematurePolls int

	//Indicates if this task is completed, e.g., executed successfully
	Completed bool
	//Indicates if this task failed
	Failed bool

	//time the task was sent to the backend
	RequestStart time.Time
	//time the task was successfully polled by the backend
	RequestCompletedAndPolled time.Time
	//time the function started
	FunctionExecutionStart int64
	//time the task execution was completed
	FunctionExecutionEnd int64
	//Time of the final poll for this task
	FinalPollTime int64
}

type PollingStrategy interface {
	/*StartJob initializes a Job, all subsequent TaskUpdates are treated
	  as related to this Job. Calling StartJob again indicates the start of a
	  new job and the end of the last job.
	*/
	StartJob(JobInfo) error

	/*
		JobUpdate updates metadata related to a job. Usually called to set the final job execution time.
	*/
	JobUpdate(JobInfo) error

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

	SetFinalPollTime(RId string, timeNano int64)

	/*used to coordinate the log creation*/
	Finalize() error
}
