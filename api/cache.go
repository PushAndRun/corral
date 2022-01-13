package api

//CacheConfigInjector interface to be used by a function to inject the cache system config into a deployment package
type CacheConfigInjector interface {
	CacheSystem() DeployableCache
}

//CacheSystem represent a ephemeral file system used for intermediate state between map/reduce phases
type CacheSystem interface {
	FileSystem
	DeployableCache

	Flush(system FileSystem) error
	Clear() error
}

type DeployableCache interface {
	//Deploy will deploy a cache based on the config and viper values - can use plugins
	Deploy() error
	//Undeploy will remove a prior deployment
	Undeploy() error
	//Check checks if the cache is deployable, e.g. if the plugin is running, all configs are set. Should not interact with the cloud provider, just locally check if everything is ready
	Check() error
	//FunctionInjector can be used by function deployment code to modify function deployments to use the underling cache system, warning needs to be implemented for each platform individually
	FunctionInjector() CacheConfigInjector
}
