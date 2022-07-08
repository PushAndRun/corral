package corral

import (
	"encoding/json"
	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/compute/corlambda"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lambda/lambdaiface"

	"github.com/stretchr/testify/assert"
)

func TestRunningInLambda(t *testing.T) {
	res := runningInLambda()
	assert.False(t, res)

	for _, env := range []string{"LAMBDA_TASK_ROOT", "AWS_EXECUTION_ENV", "LAMBDA_RUNTIME_DIR"} {
		os.Setenv(env, "value")
	}

	res = runningInLambda()
	assert.True(t, res)
}

func TestHandleRequest(t *testing.T) {
	testTask := api.Task{
		JobNumber:        0,
		Phase:            api.MapPhase,
		BinID:            0,
		IntermediateBins: 10,
		Splits:           []api.InputSplit{},
		FileSystemType:   api.Local,
		WorkingLocation:  ".",
	}

	job := &Job{
		config: &config{},
	}

	// These values should be reset to 0 by Lambda handler function
	job.bytesRead = 10
	job.bytesWritten = 20

	lambdaDriver = NewDriver(job)
	lambdaDriver.runtimeID = "foo"
	lambdaDriver.Start = time.Time{}

	mockTaskResult := api.TaskResult{
		BytesRead:    0,
		BytesWritten: 0,
		Log:          "",
		HId:          mockHostID(),
		RId:          mockHostID(),
		CId:          lambdaDriver.runtimeID,
		JId:          "0_0_0", //job_phase_bin
		CStart:       lambdaDriver.Start.UnixNano(),
		EStart:       0,
		EEnd:         0,
	}

	output, err := handle(lambdaDriver, mockHostID, mockHostID)(testTask)
	assert.Nil(t, err)
	output.EStart = 0
	output.EEnd = 0
	assert.Equal(t, mockTaskResult, output)

	testTask.Phase = api.ReducePhase
	mockTaskResult.JId = "0_1_0"
	output, err = handle(lambdaDriver, mockHostID, mockHostID)(testTask)

	assert.Nil(t, err)
	output.EStart = 0
	output.EEnd = 0
	assert.Equal(t, mockTaskResult, output)
}

func mockHostID() string {
	return "test"
}

type mockLambdaClient struct {
	lambdaiface.LambdaAPI
	capturedPayload []byte
}

func (m *mockLambdaClient) Invoke(input *lambda.InvokeInput) (*lambda.InvokeOutput, error) {
	m.capturedPayload = input.Payload
	return &lambda.InvokeOutput{}, nil
}

func (*mockLambdaClient) GetFunction(*lambda.GetFunctionInput) (*lambda.GetFunctionOutput, error) {
	return nil, nil
}

func (*mockLambdaClient) CreateFunction(*lambda.CreateFunctionInput) (*lambda.FunctionConfiguration, error) {
	return nil, nil
}

func TestRunLambdaMapper(t *testing.T) {
	mock := &mockLambdaClient{}
	executor := &lambdaExecutor{
		&corlambda.LambdaClient{
			Client: mock,
		},
		nil,
		"FunctionName",
	}

	job := &Job{
		config: &config{WorkingLocation: "."},
	}
	err := executor.RunMapper(job, 0, 10, []api.InputSplit{})
	assert.Nil(t, err)

	var taskPayload api.Task
	err = json.Unmarshal(mock.capturedPayload, &taskPayload)
	assert.Nil(t, err)

	assert.Equal(t, uint(10), taskPayload.BinID)
	assert.Equal(t, api.MapPhase, taskPayload.Phase)
}

func TestRunLambdaReducer(t *testing.T) {
	mock := &mockLambdaClient{}
	executor := &lambdaExecutor{
		&corlambda.LambdaClient{
			Client: mock,
		},
		nil,
		"FunctionName",
	}

	job := &Job{
		config: &config{WorkingLocation: "."},
	}
	err := executor.RunReducer(job, 0, 10)
	assert.Nil(t, err)

	var taskPayload api.Task
	err = json.Unmarshal(mock.capturedPayload, &taskPayload)
	assert.Nil(t, err)

	assert.Equal(t, uint(10), taskPayload.BinID)
	assert.Equal(t, api.ReducePhase, taskPayload.Phase)
}

func TestDeployFunction(t *testing.T) {
	mock := &mockLambdaClient{}
	executor := &lambdaExecutor{
		&corlambda.LambdaClient{
			Client: mock,
		},
		nil,
		"FunctionName",
	}

	viper.SetDefault("lambdaManageRole", false) // Disable testing role deployment
	executor.Deploy(&Driver{})
}
