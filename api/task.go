package api

// Phase is a descriptor of the phase (i.e. Map or Reduce) of a Job
type Phase int

// Descriptors of the Job phase
const (
	MapPhase Phase = iota
	ReducePhase
	SortMapPhase
	SortReducePhase
)

// Task defines a serialized description of a single unit of work
// in a MapReduce job, as well as the necessary information for a
// remote executor to initialize itself and begin working.
type Task struct {
	JobId            int64
	JobNumber        int
	TaskId           int64
	Phase            Phase
	BinID            uint
	IntermediateBins uint
	Splits           []InputSplit
	FileSystemType   FileSystemType
	CacheSystemType  CacheSystemType
	WorkingLocation  string
	Cleanup          bool
}

type TaskResult struct {
	TaskId int64 `json:"TaskId"` //Task identifier (used for polling Benchmarking)
	JobId  int64 `json:"JobId"`  //Job identifier (used for polling Benchmarking)

	BytesRead    int
	BytesWritten int

	Log string

	HId    string `json:"HId"`    //host identifier
	CId    string `json:"CId"`    //runtime identifier
	JId    string `json:"JId"`    //job identifier
	RId    string `json:"RId"`    //request identifier (by platform)
	CStart int64  `json:"cStart"` //start of runtime
	EStart int64  `json:"eStart"` //start of request
	EEnd   int64  `json:"eEnd"`   //end of request

	CWT int64 `json:"CWT"` //Cache WriteTime
	CRT int64 `json:"CRT"` //Cache ReadTime
	SWT int64 `json:"SWT"` //S3 WriteTime
	SRT int64 `json:"SRT"` //S3 ReadTime
}
