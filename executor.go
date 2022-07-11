package corral

import "github.com/ISE-SMILE/corral/api"

type executor interface {
	RunMapper(job *Job, jobNumber int, binID uint, inputSplits []api.InputSplit) error
	RunReducer(job *Job, jobNumber int, binID uint) error

	//HintSplits is called before running a Map/Reducer Step to give the backend a hint on needed scale
	HintSplits(splits uint) error
}

type smileExecutor interface {
	executor
	BatchRunMapper(job *Job, jobNumber int, inputSplits [][]api.InputSplit) error
	BatchRunReducer(job *Job, jobNumber int, bins []uint) error
}
