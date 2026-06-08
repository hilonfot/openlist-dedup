package scanner

// Storage represents a media storage provider.
type Storage string

const (
	StorageLocal Storage = "local"
	StorageQuark Storage = "quark"
	StorageTianyi Storage = "tianyi"
)

// ScanTask represents a directory to scan.
type ScanTask struct {
	Storage Storage
	Path    string
}

// ScanResult holds the scanned file information.
type ScanResult struct {
	Storage  Storage
	Path     string
	Name     string
	Size     int64
	IsDir    bool
	Modified string
}
