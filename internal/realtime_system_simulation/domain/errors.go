package domain

import "errors"

var (
	ErrRunNotFound      = errors.New("simulation run not found")
	ErrRunAlreadyExists = errors.New("simulation run already exists")
	ErrInvalidStatus    = errors.New("invalid run status")
)
