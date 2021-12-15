package redis_cache

import (
	"github.com/ISE-SMILE/corral/internal/corcache"
	"testing"
)

func TestRedisCacheSystem(t *testing.T) {
	rcs := &RedisBackedCache{
		DeploymentStragey: &LocalRedisDeploymentStrategy{},
	}

	corcache.RunTestCacheSystem(t, rcs)
}
