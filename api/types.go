package api

// CacheSystemType is an identifier for supported FileSystems
type CacheSystemType int

// Identifiers for supported FileSystemTypes
const (
	NoCache CacheSystemType = iota
	InMemory
	Redis
	Olric
	EFS
)

// FileSystemType is an identifier for supported FileSystems
type FileSystemType int

// Identifiers for supported FileSystemTypes
const (
	Local FileSystemType = iota
	S3
	MINIO
)
