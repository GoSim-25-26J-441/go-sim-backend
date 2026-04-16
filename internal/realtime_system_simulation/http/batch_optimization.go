package http

import (
	"bytes"
	"encoding/json"
	"errors"
)

// ErrBatchOptimizationWithOnline is returned when a request sets both optimization.batch and online=true.
var ErrBatchOptimizationWithOnline = errors.New("batch optimization cannot be combined with online optimization (online=true)")

func batchOptimizationSet(batch json.RawMessage) bool {
	s := bytes.TrimSpace(batch)
	return len(s) > 0 && !bytes.Equal(s, []byte("null"))
}

func validateBatchOptimizationInput(batch json.RawMessage, online bool) error {
	if batchOptimizationSet(batch) && online {
		return ErrBatchOptimizationWithOnline
	}
	return nil
}
