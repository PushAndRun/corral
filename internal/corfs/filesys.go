package corfs

import (
	"github.com/ISE-SMILE/corral/api"
	log "github.com/sirupsen/logrus"
	"strings"
)

// FileSystemType is an identifier for supported FileSystems
type FileSystemType int

// Identifiers for supported FileSystemTypes
const (
	Local FileSystemType = iota
	S3
	MINIO
)

// InitFilesystem intializes a filesystem of the given type
func InitFilesystem(fsType FileSystemType) (api.FileSystem, error) {
	var fs api.FileSystem
	switch fsType {
	case Local:
		log.Debug("using local fs")
		fs = &LocalFileSystem{}
	case S3:
		log.Debug("using s3 fs")
		fs = &S3FileSystem{}
	case MINIO:
		log.Debug("using minio fs")
		fs = &MinioFileSystem{}

	}

	err := fs.Init()
	if err != nil {
		log.Errorf("failed to init filesystem, %+v", err)
		return nil, err
	}
	return fs, nil
}

func FilesystemType(fs api.FileSystem) FileSystemType {
	if _, ok := fs.(*LocalFileSystem); ok {
		return Local
	}
	if _, ok := fs.(*S3FileSystem); ok {
		return S3
	}
	if _, ok := fs.(*MinioFileSystem); ok {
		return MINIO
	}
	return S3
}

// InferFilesystem initializes a filesystem by inferring its type from
// a file address.
// For example, locations starting with "s3://" will resolve to an S3
// filesystem.
func InferFilesystem(location string) api.FileSystem {
	var fs api.FileSystem
	if strings.HasPrefix(location, "s3://") {
		log.Debug("using s3 fs")
		fs = &S3FileSystem{}
	} else if strings.HasPrefix(location, "minio://") {
		log.Debug("using minio fs")
		fs = &MinioFileSystem{}
	} else {
		log.Debug("using local fs")
		fs = &LocalFileSystem{}
	}

	fs.Init()
	return fs
}
