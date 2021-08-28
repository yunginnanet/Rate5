# Rate5  
[![GoDoc](https://godoc.org/github.com/yunginnanet/?status.svg)](https://godoc.org/github.com/yunginnanet/Rate5) [![Go Report Card](https://goreportcard.com/badge/github.com/yunginnanet/Rate5)](https://goreportcard.com/report/github.com/yunginnanet/Rate5)
  
A generic ratelimitter for any golang project.    

## Short Example Implementation
```  
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
	defer func() {
		c.Conn.Close()
		println("closed: " + c.Conn.RemoteAddr().String())
	}()

	// Returns true if ratelimited
	if Rater.Check(c) {
		c.Conn.Write([]byte("too many connections"))
		return
	}
    // handle connection ... 
}
    
```  
  
## To-Do  
More documentation  
Test cases