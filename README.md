# Rate5  
[![GoDoc](https://godoc.org/github.com/yunginnanet/?status.svg)](https://godoc.org/github.com/yunginnanet/Rate5) [![Go Report Card](https://goreportcard.com/badge/github.com/yunginnanet/Rate5)](https://goreportcard.com/report/github.com/yunginnanet/Rate5)
  
A generic ratelimitter for any golang project.  
See [the docs](https://godoc.org/github.com/yunginnanet/Rate5) and the [examples](_examples/rated.go) below for more details.
  
  

## Short Example
```   
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
[...]
    
```  
  
## In-depth example
  
[Concurrent TCP Server with Rate5 Ratelimiter](_examples/rated.go)  
        
## To-Do  
More Documentation  
More To-Dos  
~~Test Cases~~
More test cases.
