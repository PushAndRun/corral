package api

//CacheConfigInjector interface to be used by a function to inject the cache system config into a deployment package
type CacheConfigInjector interface {
	CacheSystem() CacheSystem
}

//CacheSystem represent a ephemeral file system used for intermediate state between map/reduce phases
type CacheSystem interface {
	FileSystem

	Deploy() error
	Undeploy() error

	Flush(system FileSystem) error
	Clear() error

	//FunctionInjector can be used by function deployment code to modify function deploymentes to use the underling cache system, warning needs to be implmented for each platfrom induvidually
	FunctionInjector() CacheConfigInjector
}
