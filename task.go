package corral

import (
	"github.com/ISE-SMILE/corral/internal/corcache"
	"github.com/ISE-SMILE/corral/internal/corfs"
)

// Phase is a descriptor of the phase (i.e. Map or Reduce) of a Job
type Phase int

// Descriptors of the Job phase
const (
	MapPhase Phase = iota
	ReducePhase
	SortMapPhase
	SortReducePhase
)

// task defines a serialized description of a single unit of work
// in a MapReduce job, as well as the necessary information for a
// remote executor to initialize itself and begin working.
type task struct {
	JobNumber        int
	Phase            Phase
	BinID            uint
	IntermediateBins uint
	Splits           []InputSplit
	FileSystemType   corfs.FileSystemType
	CacheSystemType  corcache.CacheSystemType
	WorkingLocation  string
	Cleanup          bool
}

type taskResult struct {
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
