package session

import "errors"

var (
	ErrSessionNotFound   = errors.New("session not found")
	ErrCheckpointMissing = errors.New("checkpoint not found")
	ErrLockTimeout       = errors.New("lock acquisition timeout")
)
