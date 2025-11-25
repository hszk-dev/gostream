package repository

import "errors"

var (
	// ErrVideoNotFound is returned when a video cannot be found.
	ErrVideoNotFound = errors.New("video not found")

	// ErrDuplicateVideo is returned when attempting to create a video that already exists.
	ErrDuplicateVideo = errors.New("video already exists")
)
