package corcache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/ISE-SMILE/corral/api"
	"github.com/ISE-SMILE/corral/internal/corfs"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func IntOptional(i int) *int {
	return &i
}

func mockFile(size uint) []byte {

	buf := bytes.NewBuffer(make([]byte, 0))
	for i := uint(0); i < size; i++ {
		binary.Write(buf, binary.LittleEndian, rand.Int63())
	}

	return buf.Bytes()
}

func RunTestCacheSystem(t *testing.T, c api.CacheSystem) {
	err := c.Deploy()
	if err != nil {
		t.Fatalf("failed to deploy cache")
	}

	defer c.Undeploy()

	CacheSmokeTest(t, c)
}

func CacheSmokeTest(t *testing.T, c api.CacheSystem) {
	//We need Clear to work otherwise non of the following tests will function properly
	local := prepare(t, c, "/")
	files, err := local.ListFiles("/*")
	assert.Nil(t, err)
	assert.EqualValues(t, 10, len(files))
	local.Clear()
	files, err = local.ListFiles("/*")
	assert.Nil(t, err)
	assert.EqualValues(t, 0, len(files))

	t.Run("clear", func(t *testing.T) { test_Clear(t, c) })
	t.Run("Init", func(t *testing.T) { test_init(t, c) })
	t.Run("writer", func(t *testing.T) { test_OpenWriter(t, c) })
	t.Run("list", func(t *testing.T) { test_ListFiles(t, c) })
	t.Run("stats", func(t *testing.T) { test_Stat(t, c) })
	t.Run("reader", func(t *testing.T) { test_OpenReader(t, c) })
	t.Run("delete", func(t *testing.T) { test_Delete(t, c) })
}

func test_init(t *testing.T, local api.CacheSystem) {
	err := local.Init()
	assert.Nil(t, err)
}

//we need to test prepare before we can test any of the other functions
func test_OpenWriter(t *testing.T, local api.CacheSystem) {
	local = prepare(t, local, "/")

	files, err := local.ListFiles("/*")
	assert.Nil(t, err)
	assert.GreaterOrEqual(t, len(files), 10)
	for _, file := range files {
		assert.EqualValues(t, 5*8, file.Size)
	}

	w, err := local.OpenWriter("/test0")
	assert.Nil(t, err)
	binary.Write(w, binary.LittleEndian, uint64(0xc0ffee))
	w.Close()

	w, err = local.OpenWriter("/new")
	assert.Nil(t, err)
	binary.Write(w, binary.LittleEndian, uint64(0xc0ffee))
	w.Close()

	r, err := local.OpenReader("/new", 0)
	assert.Nil(t, err)
	var test uint64
	binary.Read(r, binary.LittleEndian, &test)
	assert.EqualValues(t, test, 0xc0ffee)
}

func prepare(t *testing.T, local api.CacheSystem, prefix string) api.CacheSystem {
	init_cache(t, local)

	writeTo(local, prefix, t, "")

	return local
}

func init_cache(t *testing.T, local api.CacheSystem) {
	assert.NotNil(t, local)
	err := local.Init()
	if err != nil {
		t.Fatalf("failed to init cache %+v", err)
	}
	err = local.Clear()
	if err != nil {
		t.Fatalf("failed to clear cache %+v", err)
	}
}

func writeTo(local api.CacheSystem, prefix string, t *testing.T, suffix string) [][]byte {
	files := make([][]byte, 0)
	for i := 0; i < 10; i++ {
		w, err := local.OpenWriter(fmt.Sprintf("%stest%d%s", prefix, i, suffix))
		assert.Nil(t, err)
		file := mockFile(5)
		files = append(files, file)
		i, err := w.Write(file)
		assert.Nil(t, err)
		assert.GreaterOrEqual(t, i, 5*8)
		err = w.Close()
		if err != nil {
			t.Fatalf("failed to close file %stest%d%s, %+v", prefix, i, suffix, err)
		}
	}
	return files
}

func test_ListFiles(t *testing.T, local api.CacheSystem) {
	local = prepare(t, local, "/")

	writeTo(local, "/test/", t, "")
	writeTo(local, "/glob/", t, ".mp4")
	writeTo(local, "/glob/", t, ".mp3")

	test := func(patter string, expted int) []api.FileInfo {
		files, err := local.ListFiles(patter)
		assert.Nil(t, err)
		assert.GreaterOrEqual(t, len(files), expted)
		return files
	}

	files := test("/*", 40)
	for _, file := range files {
		assert.EqualValues(t, 5*8, file.Size)
	}

	_ = test("/test/*", 10)
	_ = test("/glob/*", 20)
	_ = test("/glob/*.mp4", 10)
	_ = test("/glob/*.mp3", 10)
	_ = test("/glob/*.mp*", 10)

}

func test_Stat(t *testing.T, local api.CacheSystem) {
	local = prepare(t, local, "/")

	s, err := local.Stat("/test0")
	assert.Nil(t, err)
	assert.Equal(t, api.FileInfo{
		Name: "/test0",
		Size: int64(5 * 8),
	}, s)

	_, err = local.Stat("/dose not exsist")
	assert.NotNil(t, err)

}

func test_OpenReader(t *testing.T, local api.CacheSystem) {
	local = prepare(t, local, "/")
	w, err := local.OpenWriter("/testfile")
	assert.Nil(t, err)

	buf := bytes.NewBuffer(make([]byte, 0))
	err = binary.Write(buf, binary.LittleEndian, uint64(42))
	assert.Nil(t, err)
	err = binary.Write(buf, binary.LittleEndian, uint64(0xc0ffee))
	assert.Nil(t, err)

	w.Write(buf.Bytes())
	w.Close()

	r, err := local.OpenReader("/testfile", 0)
	assert.Nil(t, err)
	var vint int64
	err = binary.Read(r, binary.LittleEndian, &vint)
	assert.Nil(t, err)
	assert.EqualValues(t, 42, vint)
	err = binary.Read(r, binary.LittleEndian, &vint)
	assert.Nil(t, err)
	assert.EqualValues(t, 0xc0ffee, vint)
	err = binary.Read(r, binary.LittleEndian, &vint)
	assert.NotNil(t, err)
	r.Close()

	r, err = local.OpenReader("/testfile", 8)
	assert.Nil(t, err)
	assert.EqualValues(t, 0xc0ffee, vint)
	r.Close()

}

func test_Clear(t *testing.T, local api.CacheSystem) {
	local = prepare(t, local, "/")
	files, err := local.ListFiles("/*")
	assert.Nil(t, err)
	assert.EqualValues(t, 10, len(files))
	local.Clear()
	files, err = local.ListFiles("/*")
	assert.Nil(t, err)
	assert.EqualValues(t, 0, len(files))
}

func test_Delete(t *testing.T, local api.CacheSystem) {
	local = prepare(t, local, "/")

	test := func(e int) {
		files, err := local.ListFiles("/*")
		assert.Nil(t, err)
		assert.EqualValues(t, e, len(files))
	}

	test(10)
	err := local.Delete("/test0")
	assert.Nil(t, err)
	test(9)

	err = local.Delete("/foo")
	assert.NotNil(t, err)
	test(9)

	files, err := local.ListFiles("/*")
	for _, f := range files {
		err = local.Delete(f.Name)
		assert.Nil(t, err)
	}
	assert.Nil(t, err)
	test(0)

}

func test_Flush(t *testing.T, local api.CacheSystem) {
	init_cache(t, local)
	base := &corfs.LocalFileSystem{}
	err := base.Init()
	if err != nil {
		t.Fatalf("failed to init filebase")
	}

	prefix, err := os.MkdirTemp(os.TempDir(), "corral_cache_test")
	if err != nil {
		t.Fatalf("failed to prepare base")
	}

	files := writeTo(local, prefix, t, ".bin")

	err = local.Flush(base)
	if err != nil {
		t.Fatalf("failed to flush files")
	}

	globbedFiles, err := filepath.Glob(prefix)
	if err != nil {
		t.Fatalf("failed read flushed files")
	}

	for i, file := range globbedFiles {
		f, err := os.Open(file)
		if err != nil {
			t.Fatalf("failed read flushed file %s , %+v", file, err)
		}
		buf := make([]byte, len(files[i]))
		_, err = f.Read(buf)
		t.Fatalf("failed read flushed file %s , %+v", file, err)
		assert.Equal(t, buf, files[i])
	}
}
