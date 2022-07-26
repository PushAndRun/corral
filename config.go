package corral

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func loadConfig() {
	viper.SetConfigName("corralrc") //BUG .yaml?
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.corral")

	setupDefaults()

	err := viper.ReadInConfig()
	if err != nil {
		log.Debugf("Config Read %+v", err)
	}

	viper.SetEnvPrefix("corral")
	viper.AutomaticEnv()
}

func setupDefaults() {
	defaultSettings := map[string]interface{}{
		"lambdaFunctionName": "corral_function",
		"lambdaMemory":       1500,
		"lambdaTimeout":      180,
		"lambdaManageRole":   true,
		"lambdaRoleARN":      "", // added for compleatness is used in conjunction with lambdaManageRole
		"lambdaS3Key":        "",
		"lambdaS3Bucket":     "",
		"cleanup":            true,  //if intermediate data should be deleted after use
		"durable":            false, //Should Intermeidiate data be flushed to the filesystem (in conflict with cleanup)
		"verbose":            false,
		"veryverbose":        false,
		"splitSize":          100 * 1024 * 1024, // Default input split size is 100Mb
		"mapBinSize":         512 * 1024 * 1024, // Default map bin size is 512Mb
		"reduceBinSize":      512 * 1024 * 1024, // Default reduce bin size is 512Mb
		"maxConcurrency":     500,               // Maximum number of concurrent executors
		"workingLocation":    ".",
		"requestPerMinute":   200,
		"remoteLoggingHost":  "",
		"logName":            "activations",
		"acceptTimeout":      time.Second * 60,
		"cache":              "", //coresponse to corcache.CacheSystemType

		"cacheSize": uint64(10 * 1024 * 1024), //corosponse to corcache.Local

		"redisDeploymentType":    "", //corosponse to corcache.Redis
		"kubernetesNamespace":    "", //
		"kubernetesStorageClass": "",
		"redisPort":              nil,
		"experimentNote":         "",
	}
	setupFlags(&defaultSettings)
	for key, value := range defaultSettings {
		viper.SetDefault(key, value)
	}

	aliases := map[string]string{
		"verbose":          "v",
		"working_location": "o",
		"lambdaMemory":     "m",
	}
	for key, alias := range aliases {
		viper.RegisterAlias(alias, key)
	}
}
