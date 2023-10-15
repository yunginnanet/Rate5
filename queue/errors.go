package queue

import "errors"

var ErrQueueFull = errors.New("bounded queue is full")
