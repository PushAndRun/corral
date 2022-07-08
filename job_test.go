package corral

import (
	"encoding/json"
	"fmt"
	"github.com/ISE-SMILE/corral/api"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSplitInputRecord(t *testing.T) {
	var splitRecordTests = []struct {
		input         string
		expectedKey   string
		expectedValue string
	}{
		{"foo\tbar", "foo", "bar"},
		{"foo\tbar\tbaz", "", "foo\tbar\tbaz"},
		{"foo bar baz", "", "foo bar baz"},
		{"key without value\t", "key without value", ""},
		{"\tvalue without key", "", "value without key"},
	}

	for _, test := range splitRecordTests {
		keyVal := splitInputRecord(test.input)
		assert.Equal(t, test.expectedKey, keyVal.Key)
		assert.Equal(t, test.expectedValue, keyVal.Value)
	}
}

func TestJob_CollectMetrics(t *testing.T) {
	logName := fmt.Sprintf("activations_%s.csv", time.Now().Format("2006_01_02"))
	//backup exsisting file
	if _, err := os.Stat(logName); err == nil {
		err := os.Rename(logName, logName+".bak")
		if err != nil {
			t.Fatalf("could not move %+v", err)
		}
	} else {
		if !os.IsNotExist(err) {
			t.Fatalf("could not access logfile %+v", err)
		}
	}

	viper.Set("logname", "activations")
	job := &Job{}
	go job.CollectMetrics()

	for i := 0; i < 10; i++ {
		job.Collect(api.TaskResult{
			BytesRead:    i,
			BytesWritten: i,
			Log:          "",
			HId:          "",
			RId:          "",
			CId:          "",
			JId:          "",
			CStart:       0,
			EStart:       0,
			EEnd:         0,
		})
		<-time.After(time.Second * 3)
	}

	job.done()

	file, err := ioutil.ReadFile(logName)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(file) <= 0 {
		t.Fatal("file empty")
	}
	fmt.Println(string(file))

}

func TestGenerateRequest(t *testing.T) {
	tx := api.Task{JobNumber: 0, Phase: 0, BinID: 11, IntermediateBins: 92, FileSystemType: 2, CacheSystemType: 0, WorkingLocation: "minio://tpch/output", Cleanup: false,
		Splits: []api.InputSplit{
			{Filename: "minio://tpch/10/lineitem/lineitem.tbl_ac", StartOffset: 83886080, EndOffset: 94371839},
			{Filename: "minio://tpch/10/lineitem/lineitem.tbl_ac", StartOffset: 94371840, EndOffset: 104857599},
			{Filename: "minio://tpch/10/lineitem/lineitem.tbl_ac", StartOffset: 104857600, EndOffset: 115343359},
			{Filename: "minio://tpch/10/lineitem/lineitem.tbl_ac", StartOffset: 115343360, EndOffset: 125829119},
			{Filename: "minio://tpch/10/lineitem/lineitem.tbl_ac", StartOffset: 125829120, EndOffset: 136314879},
		},
	}
	d, _ := json.Marshal(tx)
	fmt.Printf(string(d))
}
