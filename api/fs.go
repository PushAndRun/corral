package api

import "io"

// FileSystem provides the file backend for MapReduce jobs.
// Input data is read from a file system. Intermediate and output data
// is written to a file system.
// This is abstracted to allow remote filesystems like S3 to be supported.
type FileSystem interface {
	ListFiles(pathGlob string) ([]FileInfo, error)
	Stat(filePath string) (FileInfo, error)
	OpenReader(filePath string, startAt int64) (io.ReadCloser, error)
	OpenWriter(filePath string) (io.WriteCloser, error)
	Delete(filePath string) error
	Join(elem ...string) string
	Split(path string) []string
	Init() error
}

// FileInfo provides information about a file
type FileInfo struct {
	Name string // file path
	Size int64  // file size in bytes
}
