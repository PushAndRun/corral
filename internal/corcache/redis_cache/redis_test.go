package redis_cache

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Init(t *testing.T) {
	rc, err := NewRedisBackedCache("local")

	assert.Nil(t, err)
	assert.NotNil(t, rc)
}

func Test_SmokeTest(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
		return
	}

	rc, err := NewRedisBackedCache("local")

	assert.Nil(t, err)
	assert.NotNil(t, rc)

	err = rc.Deploy()
	assert.Nil(t, err)
	if err != nil {
		t.Fatal(err)
		return
	}

	assert.NotNil(t, rc.Config)

	err = rc.Init()
	assert.Nil(t, err)
	assert.NotNil(t, rc.Client)
	if err != nil {
		t.Fatal(err)
		return
	}

	_, err = rc.ListFiles("*")
	assert.Nil(t, err)
	if err != nil {
		t.Fatal(err)
		return
	}

	err = rc.Undeploy()
	assert.Nil(t, err)
	if err != nil {
		t.Fatal(err)
		return
	}
}
