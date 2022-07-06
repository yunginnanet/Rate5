# Rate5
[![GoDoc](https://godoc.org/github.com/yunginnanet/?status.svg)](https://godoc.org/github.com/yunginnanet/Rate5) [![Go Report Card](https://goreportcard.com/badge/github.com/yunginnanet/Rate5)](https://goreportcard.com/report/github.com/yunginnanet/Rate5) [![Go](https://github.com/yunginnanet/Rate5/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/yunginnanet/Rate5/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/yunginnanet/Rate5/branch/main/graph/badge.svg?token=R7WU58G5L7)](https://codecov.io/gh/yunginnanet/Rate5)

A performant and generic ratelimitter for any golang project.
See [the docs](https://godoc.org/github.com/yunginnanet/Rate5) and the unit tests as well as the below example for more details.

## Basic Example
```go
import rate5 "github.com/yunginnanet/rate5"

var Rater *rate5.Limiter

[...]
type Client struct {
        ID   string
        Conn net.Conn

        loggedin  bool
}

// Rate5 doesn't care where you derive the string used for ratelimiting
func (c Client) UniqueKey() string {
        if !c.loggedin {
                host, _, _ := net.SplitHostPort(c.Conn.RemoteAddr().String())
                return host
        }
        return c.ID
}

func (s *Server) handleTCP(c *Client) {
	// Returns true if ratelimited
	if Rater.Check(c) {
		c.Conn.Write([]byte("too many connections"))
		c.Conn.Close()
		return
	}
// [...]
}
```

## License

```
MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
