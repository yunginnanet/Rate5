package speedometer

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var ErrLimitReached = errors.New("limit reached")

// Speedometer is an io.Writer wrapper that will limit the rate at which data is written to the underlying target.
//
// It is safe for concurrent use, but writers will block when slowed down.
//
// Optionally, it can be given;
//
//   - a capacity, which will cause it to return an error if the capacity is exceeded.
//
//   - a speed limit, causing slow downs of data written to the underlying writer if the speed limit is exceeded.
type Speedometer struct {
	ceiling    int64
	speedLimit *SpeedLimit
	internal   atomics
	w          io.Writer
	r          io.Reader
	c          io.Closer
}

type atomics struct {
	count    *atomic.Int64
	closed   *atomic.Bool
	start    *sync.Once
	stop     *sync.Once
	birth    *atomic.Pointer[time.Time]
	duration *atomic.Pointer[time.Duration]
}

func newAtomics() atomics {
	manhattan := atomics{
		count:    new(atomic.Int64),
		closed:   new(atomic.Bool),
		start:    new(sync.Once),
		stop:     new(sync.Once),
		birth:    new(atomic.Pointer[time.Time]),
		duration: new(atomic.Pointer[time.Duration]),
	}
	manhattan.birth.Store(&time.Time{})
	manhattan.closed.Store(false)
	manhattan.count.Store(0)
	return manhattan
}

// SpeedLimit is used to limit the rate at which data is written to the underlying writer.
type SpeedLimit struct {
	// Burst is the number of bytes that can be written to the underlying writer per Frame.
	Burst int64
	// Frame is the duration of the frame in which Burst can be written to the underlying writer.
	Frame time.Duration
	// CheckEveryBytes is the number of bytes written before checking if the speed limit has been exceeded.
	CheckEveryBytes int64
	// Delay is the duration to delay writing if the speed limit has been exceeded during a Write call. (blocking)
	Delay time.Duration
}

func NewBytesPerSecondLimit(bytes int64) *SpeedLimit {
	return &SpeedLimit{
		Burst:           bytes,
		Frame:           time.Second,
		CheckEveryBytes: 1,
		Delay:           100 * time.Millisecond,
	}
}

const fallbackDelay = 100

func regulateSpeedLimit(speedLimit *SpeedLimit) (*SpeedLimit, error) {
	if speedLimit.Burst <= 0 || speedLimit.Frame <= 0 {
		return nil, errors.New("invalid speed limit")
	}
	if speedLimit.CheckEveryBytes <= 0 {
		speedLimit.CheckEveryBytes = speedLimit.Burst
	}
	if speedLimit.Delay <= 0 {
		speedLimit.Delay = fallbackDelay * time.Millisecond
	}
	return speedLimit, nil
}

func newSpeedometer(target any, speedLimit *SpeedLimit, ceiling int64) (*Speedometer, error) {
	var err error
	if speedLimit != nil {
		if speedLimit, err = regulateSpeedLimit(speedLimit); err != nil {
			return nil, err
		}
	}

	spd := &Speedometer{
		ceiling:    ceiling,
		speedLimit: speedLimit,
		internal:   newAtomics(),
	}

	switch t := target.(type) {
	case io.ReadWriteCloser:
		spd.w = t
		spd.r = t
		spd.c = t
	case io.ReadWriter:
		spd.w = t
		spd.r = t
	case io.WriteCloser:
		spd.w = t
		spd.c = t
	case io.ReadCloser:
		spd.r = t
		spd.c = t
	case io.Writer:
		spd.w = t
	case io.Reader:
		spd.r = t
	default:
		return nil, errors.New("invalid target")
	}

	return spd, nil
}

// NewSpeedometer creates a new Speedometer that wraps the given io.Writer.
// It will not limit the rate at which data is written to the underlying writer, it only measures it.
func NewSpeedometer(w io.Writer) (*Speedometer, error) {
	return newSpeedometer(w, nil, -1)
}

func NewReadingSpeedometer(r io.Reader) (*Speedometer, error) {
	return newSpeedometer(r, nil, -1)
}

// NewLimitedSpeedometer creates a new Speedometer that wraps the given io.Writer.
// If the speed limit is exceeded, writes to the underlying writer will be limited.
// See SpeedLimit for more information.
func NewLimitedSpeedometer(w io.Writer, speedLimit *SpeedLimit) (*Speedometer, error) {
	return newSpeedometer(w, speedLimit, -1)
}

// NewCappedSpeedometer creates a new Speedometer that wraps the given io.Writer.
// If len(written) bytes exceeds cap, writes to the underlying writer will be ceased permanently for the Speedometer.
func NewCappedSpeedometer(w io.Writer, capacity int64) (*Speedometer, error) {
	return newSpeedometer(w, nil, capacity)
}

// NewCappedLimitedSpeedometer creates a new Speedometer that wraps the given io.Writer.
// It is a combination of NewLimitedSpeedometer and NewCappedSpeedometer.
func NewCappedLimitedSpeedometer(w io.Writer, speedLimit *SpeedLimit, capacity int64) (*Speedometer, error) {
	return newSpeedometer(w, speedLimit, capacity)
}

func (s *Speedometer) increment(inc int64) (int, error) {
	if s.internal.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	var err error
	if s.ceiling > 0 && s.Total()+inc > s.ceiling {
		_ = s.Close()
		err = ErrLimitReached
		inc = s.ceiling - s.Total()
	}
	s.internal.count.Add(inc)
	return int(inc), err
}

// Running returns true if the Speedometer is still running.
func (s *Speedometer) Running() bool {
	return !s.internal.closed.Load()
}

// Total returns the total number of bytes written to the underlying writer.
func (s *Speedometer) Total() int64 {
	return s.internal.count.Load()
}

// Close stops the Speedometer. No additional writes will be accepted.
func (s *Speedometer) Close() error {
	if s.internal.closed.Load() {
		return io.ErrClosedPipe
	}

	var err error

	s.internal.stop.Do(func() {
		s.internal.closed.Store(true)
		stopped := time.Now()
		birth := s.internal.birth.Load()
		duration := stopped.Sub(*birth)
		s.internal.duration.Store(&duration)
		if s.c != nil {
			if cErr := s.c.Close(); cErr != nil && !errors.Is(cErr, net.ErrClosed) {
				err = cErr
			}
		}
	})

	return err
}

// Rate returns the bytes per second rate at which data is being written to the underlying writer.
func (s *Speedometer) Rate() float64 {
	if s.internal.closed.Load() {
		return 0
	}
	return float64(s.Total()) / time.Since(*s.internal.birth.Load()).Seconds()
}

func (s *Speedometer) slowDown() error {
	switch {
	case s.speedLimit == nil:
		// welcome to the autobahn, motherfucker.
		return nil
	case s.speedLimit.Burst <= 0 || s.speedLimit.Frame <= 0,
		s.speedLimit.CheckEveryBytes <= 0, s.speedLimit.Delay <= 0:
		// invalid speedLimit
		return errors.New("invalid speed limit")
	case s.Total()%int64(s.speedLimit.CheckEveryBytes) != 0:
		// if (total written [modulus] checkeverybytes is not 0) then our total byte count
		// is not a multiple of our configured check frequency.
		// bypass check and write at normal speed
		return nil
	default:
		//
	}

	// the slowing will continue until morale improves
	// (sleep until our overall rate re-enters acceptable threshhold)
	for s.Rate() > float64(s.speedLimit.Burst)/s.speedLimit.Frame.Seconds() {
		time.Sleep(s.speedLimit.Delay)
	}

	return nil
}

var (
	ErrWriteOnly = errors.New("not a reader")
	ErrReadOnly  = errors.New("not a writer")
)

type ioType int

const (
	ioWriter ioType = iota
	ioReader
)

type actor func(p []byte) (n int, err error)

func (s *Speedometer) chkIOType(t ioType) (actor, error) {
	if s.internal.closed.Load() {
		return nil, io.ErrClosedPipe
	}

	switch t {
	case ioWriter:
		if s.w == nil {
			return nil, ErrReadOnly
		}
		return s.w.Write, nil
	case ioReader:
		if s.r == nil {
			return nil, ErrWriteOnly
		}
		return s.r.Read, nil
	default:
		panic("invalid ioType")
	}

}

func (s *Speedometer) do(t ioType, p []byte) (n int, err error) {
	var ioActor actor
	if ioActor, err = s.chkIOType(t); err != nil {
		return 0, err
	}

	s.internal.start.Do(func() {
		now := time.Now()
		s.internal.birth.Store(&now)
	})

	// if no speed limit, just write and record
	if s.speedLimit == nil {
		n, err = ioActor(p)
		if err != nil {
			return n, fmt.Errorf("error writing to underlying writer: %w", err)
		}
		return s.increment(int64(len(p)))
	}

	var (
		wErr     error
		accepted int
	)
	accepted, wErr = s.increment(int64(len(p)))

	if wErr != nil {
		return 0, fmt.Errorf("error incrementing: %w", wErr)
	}

	_ = s.slowDown()

	var iErr error
	if n, iErr = ioActor(p[:accepted]); iErr != nil {
		return n, fmt.Errorf("error writing to underlying writer: %w", iErr)
	}
	return
}

// Write writes p to the underlying writer, following all defined speed limits.
func (s *Speedometer) Write(p []byte) (n int, err error) {
	return s.do(ioWriter, p)
}

func (s *Speedometer) Read(p []byte) (n int, err error) {
	return s.do(ioReader, p)
}
