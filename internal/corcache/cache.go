package corcache

import (
	"fmt"
	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/internal/corcache/redis_cache"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// NewCacheSystem initializes a CacheSystem of the given type
func NewCacheSystem(fsType api.CacheSystemType) (api.CacheSystem, error) {
	var cs api.CacheSystem

	switch fsType {
	//this implies that
	case api.NoCache:
		log.Info("No CacheSystem availible, using FileSystme as fallback")
		return nil, nil
	case api.InMemory:
		//TODO: make this configururable
		cs = NewLocalInMemoryProvider(viper.GetUint64("cacheSize"))
	case api.Redis:
		var err error
		cs, err = redis_cache.NewRedisBackedCache(viper.GetString("redisDeploymentType"))
		if err != nil {
			log.Debugf("failed to init redis cache, %+v", err)
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unknown cache type or not yet implemented %d", fsType)
	}

	return cs, nil
}

// CacheSystemTypes returns a CacheSystemType for a given CacheSystem pointer or the NoCache type.
func CacheSystemTypes(fs api.CacheSystem) api.CacheSystemType {
	if fs == nil {
		return api.NoCache
	}

	if _, ok := fs.(*LocalCache); ok {
		return api.InMemory
	}

	if _, ok := fs.(*redis_cache.RedisBackedCache); ok {
		return api.Redis
	}

	return api.NoCache
}
