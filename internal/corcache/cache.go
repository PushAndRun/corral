package corcache

import (
	"fmt"
	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/internal/corcache/redis_cache"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// FileSystemType is an identifier for supported FileSystems
type CacheSystemType int

// Identifiers for supported FileSystemTypes
const (
	NoCache CacheSystemType = iota
	Local
	Redis
	Olric
	EFS
)

// NewCacheSystem intializes a CacheSystem of the given type
func NewCacheSystem(fsType CacheSystemType) (api.CacheSystem, error) {
	var cs api.CacheSystem

	switch fsType {
	//this implies that
	case NoCache:
		log.Info("No CacheSystem availible, using FileSystme as fallback")
		return nil, nil
	case Local:
		//TODO: make this configururable
		cs = NewLocalInMemoryProvider(viper.GetUint64("cacheSize"))
	case Redis:
		var err error
		cs, err = redis_cache.NewRedisBackedCache(redis_cache.DeploymentType(viper.GetInt("redisDeploymentType")))
		if err != nil {
			log.Debugf("failed to init redis cache, %+v", err)
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unknown cache type or not yet implemented %d", fsType)
	}

	return cs, nil
}

// CacheSystemTypes retunrs a type for a given CacheSystem or the NoCache type.
func CacheSystemTypes(fs api.CacheSystem) CacheSystemType {
	if fs == nil {
		return NoCache
	}

	if _, ok := fs.(*LocalCache); ok {
		return Local
	}
	return NoCache
}
