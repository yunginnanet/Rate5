package speedometer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testWriter struct {
	t     *testing.T
	total int64
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	atomic.AddInt64(&w.total, int64(len(p)))
	return len(p), nil
}

func (w *testWriter) Read(p []byte) (n int, err error) {
	atomic.AddInt64(&w.total, int64(len(p)))
	return len(p), nil
}

func writeStuff(t *testing.T, target io.Writer, count int) error {
	t.Helper()
	write := func() error {
		_, err := target.Write([]byte("a"))
		if err != nil {
			return fmt.Errorf("error writing: %w", err)
		}
		return nil
	}

	if count < 0 {
		var err error
		for err = write(); err == nil; err = write() {
			time.Sleep(5 * time.Millisecond)
		}
		return err
	}
	for i := 0; i < count; i++ {
		if err := write(); err != nil {
			return err
		}
	}
	return nil
}

func readStuff(t *testing.T, target io.Reader, count int) error {
	t.Helper()
	read := func() error {
		_, err := target.Read(make([]byte, 1))
		if err != nil {
			return fmt.Errorf("error reading: %w", err)
		}
		return nil
	}

	if count < 0 {
		var err error
		for err = read(); err == nil; err = read() {
			time.Sleep(5 * time.Millisecond)
		}
		return err
	}
	for i := 0; i < count; i++ {
		if err := read(); err != nil {
			return err
		}
	}
	return nil
}

type results struct {
	total   int64
	written int
	rate    float64
	err     error
}

func isIt(want, have results, t *testing.T) {
	t.Helper()
	if have.total != want.total {
		t.Errorf("total: want %d, have %d", want.total, have.total)
	}
	if have.written != want.written {
		t.Errorf("written: want %d, have %d", want.written, have.written)
	}
	if have.rate != want.rate {
		t.Errorf("rate: want %f, have %f", want.rate, have.rate)
	}
	if !errors.Is(have.err, want.err) {
		t.Errorf("wantErr: want %v, have %v", want.err, have.err)
	}
}

//nolint:funlen
func Test_Speedometer(t *testing.T) {
	t.Parallel()

	var (
		errChan = make(chan error, 10)
	)

	t.Run("EarlyClose", func(t *testing.T) {
		t.Parallel()
		t.Run("Write", func(t *testing.T) {
			var (
				err error
				cnt int
			)
			t.Parallel()
			sp, nerr := NewSpeedometer(&testWriter{t: t})
			if nerr != nil {
				t.Errorf("unexpected error: %v", nerr)
			}
			go func() {
				errChan <- writeStuff(t, sp, -1)
			}()
			time.Sleep(1 * time.Second)
			if closeErr := sp.Close(); closeErr != nil {
				t.Errorf("wantErr: want %v, have %v", nil, closeErr)
			}
			err = <-errChan
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("wantErr: want %v, have %v", io.ErrClosedPipe, err)
			}
			cnt, err = sp.Write([]byte("a"))
			isIt(results{err: io.ErrClosedPipe, written: 0}, results{err: err, written: cnt}, t)
		})
		t.Run("Read", func(t *testing.T) {
			var (
				err error
				cnt int
			)
			t.Parallel()
			sp, nerr := NewSpeedometer(&testWriter{t: t})
			if nerr != nil {
				t.Errorf("unexpected error: %v", nerr)
			}
			go func() {
				errChan <- readStuff(t, sp.r, -1)
			}()
			time.Sleep(1 * time.Second)
			if closeErr := sp.Close(); closeErr != nil {
				t.Errorf("wantErr: want %v, have %v", nil, closeErr)
			}
			t.Run("ZeroRateAfterClose", func(t *testing.T) {
				if sp.Rate() != 0 {
					t.Errorf("rate: want %f after 'Close' method, have %f", 0.0, sp.Rate())
				}
			})
			err = <-errChan
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("wantErr: want %v, have %v", io.ErrClosedPipe, err)
			}
			cnt, err = sp.Read(make([]byte, 1))
			isIt(results{err: io.ErrClosedPipe, written: 0}, results{err: err, written: cnt}, t)
		})
	})

	t.Run("EarlyCloseReader", func(t *testing.T) {
		var (
			err error
			cnt int
		)
		t.Parallel()
		sp, nerr := NewSpeedometer(&testWriter{t: t})
		if nerr != nil {
			t.Errorf("unexpected error: %v", nerr)
		}
		go func() {
			errChan <- readStuff(t, sp, -1)
		}()
		time.Sleep(1 * time.Second)
		if closeErr := sp.Close(); closeErr != nil {
			t.Errorf("wantErr: want %v, have %v", nil, closeErr)
		}
		err = <-errChan
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("wantErr: want %v, have %v", io.ErrClosedPipe, err)
		}
		cnt, err = sp.Read(make([]byte, 1))
		isIt(results{err: io.ErrClosedPipe, written: 0}, results{err: err, written: cnt}, t)
	})

	t.Run("Basic", func(t *testing.T) {
		var (
			err error
			cnt int
		)
		t.Parallel()
		sp, nerr := NewSpeedometer(&testWriter{t: t})
		if nerr != nil {
			t.Errorf("unexpected error: %v", nerr)
		}
		cnt, err = sp.Write([]byte("a"))
		isIt(results{err: nil, written: 1, total: 1}, results{err: err, written: cnt, total: sp.Total()}, t)
		cnt, err = sp.Write([]byte("aa"))
		isIt(results{err: nil, written: 2, total: 3}, results{err: err, written: cnt, total: sp.Total()}, t)
		cnt, err = sp.Write([]byte("a"))
		isIt(results{err: nil, written: 1, total: 4}, results{err: err, written: cnt, total: sp.Total()}, t)
		cnt, err = sp.Write([]byte("a"))
		isIt(results{err: nil, written: 1, total: 5}, results{err: err, written: cnt, total: sp.Total()}, t)
	})

	t.Run("ConcurrentWrites", func(t *testing.T) {
		var (
			err error
		)

		count := int64(0)
		sp, nerr := NewSpeedometer(&testWriter{t: t})
		if nerr != nil {
			t.Errorf("unexpected error: %v", nerr)
		}
		wg := &sync.WaitGroup{}
		wg.Add(100)
		for i := 0; i < 100; i++ {
			go func() {
				var counted int
				var gerr error
				counted, gerr = sp.Write([]byte("a"))
				if gerr != nil {
					t.Errorf("unexpected error: %v", err)
				}
				atomic.AddInt64(&count, int64(counted))
				wg.Done()
			}()
		}
		wg.Wait()
		isIt(results{err: nil, written: 100, total: 100},
			results{err: err, written: int(atomic.LoadInt64(&count)), total: sp.Total()}, t)
	})

	t.Run("GottaGoFast", func(t *testing.T) {
		t.Parallel()
		var (
			err error
		)
		sp, nerr := NewSpeedometer(&testWriter{t: t})
		if nerr != nil {
			t.Errorf("unexpected error: %v", nerr)
		}
		go func() {
			errChan <- writeStuff(t, sp, -1)
		}()
		var count = 0
		for sp.Running() {
			select {
			case err = <-errChan:
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("unexpected error: %v", err)
				} else {
					if count < 5 {
						t.Errorf("too few iterations: %d", count)
					}
					t.Logf("final rate: %v per second", sp.Rate())
				}
			default:
				if count > 5 {
					_ = sp.Close()
				}
				time.Sleep(100 * time.Millisecond)
				t.Logf("rate: %v per second", sp.Rate())
				count++
			}
		}
	})

	// test limiter with speedlimit
	t.Run("CantGoFast", func(t *testing.T) {
		t.Parallel()
		t.Run("10BytesASecond", func(t *testing.T) {
			t.Parallel()
			var (
				err error
			)
			sp, nerr := NewLimitedSpeedometer(&testWriter{t: t}, &SpeedLimit{
				Burst:           10,
				Frame:           time.Second,
				CheckEveryBytes: 1,
				Delay:           100 * time.Millisecond,
			})
			if nerr != nil {
				t.Errorf("unexpected error: %v", nerr)
			}
			for i := 0; i < 15; i++ {
				if _, err = sp.Write([]byte("a")); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				/*if sp.IsSlow() {
					t.Errorf("unexpected slow state")
				}*/
				t.Logf("rate: %v per second", sp.Rate())
				if sp.Rate() > 10 {
					t.Errorf("speeding in a school zone (expected under %d): %v", sp.speedLimit.Burst, sp.Rate())
				}
			}
		})

		t.Run("1000BytesPer5SecondsMeasuredEvery5000Bytes", func(t *testing.T) {
			t.Parallel()
			var (
				err error
			)
			sp, nerr := NewLimitedSpeedometer(&testWriter{t: t}, &SpeedLimit{
				Burst:           1000,
				Frame:           2 * time.Second,
				CheckEveryBytes: 5000,
				Delay:           500 * time.Millisecond,
			})

			if nerr != nil {
				t.Errorf("unexpected error: %v", nerr)
			}

			for i := 0; i < 4999; i++ {
				if _, err = sp.Write([]byte("a")); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if i%1000 == 0 {
					t.Logf("rate: %v per second", sp.Rate())
				}
				if sp.Rate() < 1000 {
					t.Errorf("shouldn't have slowed down yet (expected over %d): %v", sp.speedLimit.Burst, sp.Rate())
				}
			}
			if _, err = sp.Write([]byte("a")); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			for i := 0; i < 10; i++ {
				if _, err = sp.Write([]byte("a")); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				t.Logf("rate: %v per second", sp.Rate())
				if sp.Rate() > 1000 {
					t.Errorf("speeding in a school zone (expected under %d): %v", sp.speedLimit.Burst, sp.Rate())
				}
			}
		})
	})

	// test capped speedometer
	t.Run("OnlyALittle", func(t *testing.T) {
		t.Parallel()
		var (
			err error
		)
		sp, nerr := NewCappedSpeedometer(&testWriter{t: t}, 1024)
		if nerr != nil {
			t.Errorf("unexpected error: %v", nerr)
		}
		for i := 0; i < 1024; i++ {
			if _, err = sp.Write([]byte("a")); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if sp.Total() > 1024 {
				t.Errorf("shouldn't have written more than 1024 bytes")
			}
		}
		if _, err = sp.Write([]byte("a")); err == nil {
			t.Errorf("expected error when writing over capacity")
		}
	})

	t.Run("limited TCP bandwidth", func(t *testing.T) {
		t.Parallel()
		var (
			server net.Listener
			err    error
		)
		//goland:noinspection GoCommentLeadingSpace
		if server, err = net.Listen("tcp", ":8080"); err != nil { // #nosec:G102 - this is a unit test.
			t.Fatalf("Failed to start server: %v", err)
		}
		defer func(server net.Listener) {
			if cErr := server.Close(); cErr != nil {
				t.Errorf("failed to close server: %v", cErr)
			}
		}(server)

		go func() {
			var (
				conn net.Conn
				aErr error
			)
			if conn, aErr = server.Accept(); aErr != nil {
				t.Errorf("failed to accept connection: %v", aErr)
			}

			t.Logf("accepted connection from %s", conn.RemoteAddr().String())

			defer func(conn net.Conn) {
				if cErr := conn.Close(); cErr != nil &&
					!strings.Contains(cErr.Error(), "use of closed network connection") {
					t.Errorf("failed to close connection: %v", cErr)
				}
			}(conn)

			speedLimit := &SpeedLimit{
				Burst:           512,
				Frame:           time.Second,
				CheckEveryBytes: 1,
				Delay:           10 * time.Millisecond,
			}

			var (
				speedometer *Speedometer
				sErr        error
			)
			if speedometer, sErr = NewCappedLimitedSpeedometer(conn, speedLimit, 4096); sErr != nil {
				t.Errorf("failed to create speedometer: %v", sErr)
			}

			buf := make([]byte, 1024)
			for i := range buf {
				targ := byte('E')
				if i%2 == 0 {
					targ = byte('e')
				}
				buf[i] = targ
			}
			for {
				n, wErr := speedometer.Write(buf)
				switch {
				case errors.Is(wErr, io.EOF), errors.Is(wErr, ErrLimitReached):
					return
				case wErr != nil:
					t.Errorf("failed to write: %v", wErr)
				case n != len(buf):
					t.Errorf("failed to write all bytes: %d", n)
				default:
					t.Logf("wrote %d bytes (rate: %v/bps)", n, speedometer.Rate())
				}
			}
		}()

		var (
			client net.Conn
			aErr   error
		)

		if client, aErr = net.Dial("tcp", "localhost:8080"); aErr != nil {
			t.Fatalf("failed to connect to server: %v", err)
		}

		defer func(client net.Conn) {
			if clErr := client.Close(); clErr != nil {
				t.Errorf("failed to close client: %v", err)
			}
		}(client)

		buf := &bytes.Buffer{}
		startTime := time.Now()
		n, cpErr := io.Copy(buf, client)
		if cpErr != nil {
			t.Errorf("failed to copy: %v", cpErr)
		}

		duration := time.Since(startTime)
		if buf.Len() == 0 || n == 0 {
			t.Fatalf("nNo data received")
		}

		rate := measureRate(t, n, duration)

		if rate > 512.0 {
			t.Fatalf("rate exceeded: got %f, expected <= 100.0", rate)
		}
	})
}

type badWrites struct {
}

func (bw badWrites) Write(_ []byte) (int, error) {
	return 0, io.EOF
}

func (bw badWrites) Close() error { return io.ErrNoProgress }

func TestImprobableEdgeCasesForCoverage(t *testing.T) {
	t.Parallel()
	sp, _ := NewSpeedometer(io.Discard)
	sp.speedLimit = nil
	if sp.slowDown() != nil {
		t.Fatal("should have received no error when running slowdown with nil speedlimit")
	}
	sp, _ = NewLimitedSpeedometer(io.Discard, NewBytesPerSecondLimit(5))
	sp.speedLimit.Burst = 0
	if sp.slowDown() == nil {
		t.Fatal("should have received error when running slowdown with invalid speedlimit")
	}
	sp, _ = NewLimitedSpeedometer(badWrites{}, NewBytesPerSecondLimit(5))
	if _, e := sp.Write([]byte("yeet")); !errors.Is(e, io.EOF) {
		t.Errorf("wrong error from underlying writer err passdown: %v", e)
	}
	sp.speedLimit = nil
	if _, e := sp.Write([]byte("yeet")); !errors.Is(e, io.EOF) {
		t.Errorf("wrong error from underlying writer err passdown: %v", e)
	}
	if e := sp.Close(); e == nil {
		t.Fatal("should have received error when closing with bad writer")
	}
	if e := sp.Close(); !errors.Is(e, io.ErrClosedPipe) {
		t.Errorf("wrong error from already closed speedo: %v", e)
	}
	if _, e := sp.increment(1); !errors.Is(e, io.ErrClosedPipe) {
		t.Errorf("wrong error from already closed speedo: %v", e)
	}
	if _, err := NewLimitedSpeedometer(nil, nil); err == nil {
		t.Fatal("should have received error when creating invalid speedo")
	}
	if _, err := NewLimitedSpeedometer(io.Discard, &SpeedLimit{}); err == nil {
		t.Fatal("should have received error when creating invalid speedo")
	}
	sl := NewBytesPerSecondLimit(5)
	sl.CheckEveryBytes = 0
	sl.Delay = 0
	var err error
	if sp, err = NewLimitedSpeedometer(io.Discard, sl); err != nil {
		t.Fatal("should have received no error when creating iffy speedo")
	}
	if sp.speedLimit.CheckEveryBytes != 5 {
		t.Fatal("speed limit regularization failed")
	}
	if sp.speedLimit.Delay != time.Duration(100)*time.Millisecond {
		t.Fatal("speed limit regularization failed")
	}
}

type writeCloser struct{}

func (wc writeCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (wc writeCloser) Close() error {
	return nil
}

type readCloser struct{}

func (rc readCloser) Read(p []byte) (int, error) {
	return len(p), nil
}

func (rc readCloser) Close() error {
	return nil
}

type reader struct{}

func (r reader) Read(p []byte) (int, error) {
	return len(p), nil
}

func TestMiscellaneousBehaviorForCoverage(t *testing.T) {
	sp, err := NewSpeedometer(writeCloser{})
	if err != nil || sp == nil {
		t.Fatal("unexpected error")
	}
	if act, actErr := sp.chkIOType(ioReader); act != nil || actErr == nil {
		t.Fatal("should have received error when checking for reader on writecloser")
	}
	if sp.w == nil {
		t.Fatal("unexpected nil writer")
	}
	if sp.r != nil {
		t.Fatal("unexpected reader")
	}
	if sp.c == nil {
		t.Fatal("unexpected nil closer")
	}
	sp, err = NewReadingSpeedometer(reader{})
	if err != nil || sp == nil {
		t.Fatal("unexpected error")
	}
	if act, actErr := sp.chkIOType(ioWriter); act != nil || actErr == nil {
		t.Fatal("should have received error when checking for writer on readcloser")
	}
	if sp.w != nil {
		t.Fatal("unexpected writer")
	}
	if sp.r == nil {
		t.Fatal("unexpected nil reader")
	}
	if sp.c != nil {
		t.Fatal("unexpected closer")
	}
	sp, err = NewReadingSpeedometer(readCloser{})
	if err != nil || sp == nil {
		t.Fatal("unexpected error")
	}
	if sp.w != nil {
		t.Fatal("unexpected writer")
	}
	if sp.r == nil {
		t.Fatal("unexpected nil reader")
	}
	if sp.c == nil {
		t.Fatal("unexpected nil closer")
	}
}

func TestMustPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	sp, _ := NewSpeedometer(writeCloser{})
	_, _ = sp.chkIOType(ioType(55))
}

func measureRate(t *testing.T, received int64, duration time.Duration) float64 {
	t.Helper()
	return float64(received) / duration.Seconds()
}
