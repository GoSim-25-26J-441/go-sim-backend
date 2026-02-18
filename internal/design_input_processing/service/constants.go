package service

import "time"

const (
	// DefaultTimeout is the standard timeout for most upstream operations
	DefaultTimeout = 30 * time.Second
	
	// LongTimeout is for operations that may take longer (export, ingest)
	LongTimeout = 90 * time.Second
	
	// FuseTimeout is for fuse operations which can take several minutes
	FuseTimeout = 3 * time.Minute
)
