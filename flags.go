package corral

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var flags = []string{
	//orchestration
	//ready
	"callback", //uses a function callback feature to collect invocation results
	//todo
	"proactive", //proactively send activation based on prior observations
	//ready
	"eventBatching", //batching events

	//mittegations
	//prepared
	"multiDeploy", //uses multiple different-sized deployments, for straggler mitigation
	//todo
	"stageResizing", //resizes deployments based on current stage performance

	//compostions
	//todo
	"scheduling", // use composition-aware scheduling
	//ready
	"hinting", // send hints about invocations beforehand

	//data io
	//ready
	"caching", //use selective caching to speedup stage computations
	//todo
	"preloading", //preload tables using life-cycle-hooks
}

//Package containing a collection of experimental flags that can be used during operations
func setupFlags(defaults *map[string]interface{}) {

	for _, key := range flags {
		(*defaults)[key] = false
	}

	defaultConfig := map[string]interface{}{
		//eventBatching
		"eventBatchSize": 32,

		//multiDeploy
		"numMultiDeployments": 3,
		"maxMemory":           2048,

		//callback
		"ip": "",
	}

	for key, value := range defaultConfig {
		(*defaults)[key] = value
	}
}

//CompileFlagName all set flags and compile a unique identifier
func CompileFlagName() string {
	flag := int32(0)

	for i, key := range flags {
		if viper.GetBool(key) {
			flag = flag | 1<<i
		}
	}

	return fmt.Sprintf("%010X", flag)
}

//validateFlagConfig checks the present config including the set flegs to check if the request behavior is possible or if some settings break the requested feature
func validateFlagConfig(driver *Driver) bool {
	if viper.GetBool("scheduling") || viper.GetBool("callback") || viper.GetBool("eventBatching") || viper.GetBool("hinting") || viper.GetBool("preloading") {
		if driver.executor != nil && driver.config.Backend != "whisk" {
			log.Debug("can't use [scheduling,callback,eventBatching,hinting,preloading] flag(s) without Whisk backend")
			return false
		}
	}

	if viper.GetBool("eventBatching") {
		if driver.executor != nil {
			if _, ok := driver.executor.(smileExecutor); !ok {
				log.Debug("executor must support batching for event batching flag.")
				return false
			}
		}
	}
	return true
}
