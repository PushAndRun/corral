package corfs

import (
	"github.com/ISE-SMILE/corral/api"
	log "github.com/sirupsen/logrus"
	"strings"
)

// InitFilesystem intializes a filesystem of the given type
func InitFilesystem(fsType api.FileSystemType) (api.FileSystem, error) {
	var fs api.FileSystem
	switch fsType {
	case api.Local:
		log.Debug("using local fs")
		fs = &LocalFileSystem{}
	case api.S3:
		log.Debug("using s3 fs")
		fs = &S3FileSystem{}
	case api.MINIO:
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

func FilesystemType(fs api.FileSystem) api.FileSystemType {
	if _, ok := fs.(*LocalFileSystem); ok {
		return api.Local
	}
	if _, ok := fs.(*S3FileSystem); ok {
		return api.S3
	}
	if _, ok := fs.(*MinioFileSystem); ok {
		return api.MINIO
	}
	return api.S3
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
