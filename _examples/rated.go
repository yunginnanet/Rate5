package main

import (
	"bufio"
	"encoding/base64"
	rate5 "github.com/yunginnanet/Rate5"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Map     map[string]*Client
	AuthLog map[string][]Login
	mu      *sync.RWMutex
}

type Login struct {
	IP   string
	Time time.Time
}

type Client struct {
	ID   string
	Conn net.Conn

	loggedin  bool
	connected bool
	autlog    []Login

	deadline time.Duration
	read     *bufio.Reader
}

// Rate5 doesn't care where you derive the string used for ratelimiting
func (c Client) UniqueKey() string {
	if !c.loggedin {
		host, _, _ := net.SplitHostPort(c.Conn.RemoteAddr().String())
		return host
	}
	return c.ID
}

var (
	// Rater is our connection ratelimiter using default limiter settings.
	Rater *rate5.Limiter
	// RegRater will only allow one registration per 50 seconds and will add to the wait each time you get limited. (by IP)
	RegRater *rate5.Limiter
	// CmdRater will slow down commands sent, if not logged in by IP, if logged in by ID.
	CmdRater *rate5.Limiter

	srv     *Server
	keySize int = 8
)

func (s *Server) handleTCP(c *Client) {
	c.Conn.(*net.TCPConn).SetLinger(0)
	defer func() {
		c.Conn.Close()
		println("closed: " + c.Conn.RemoteAddr().String())
	}()

	// Returns true if ratelimited
	if Rater.Check(c) {
		c.Conn.Write([]byte("too many connections"))
		println(c.UniqueKey() + " ratelimited")
		return
	}

	c.read = bufio.NewReader(c.Conn)

	c.Conn.Write(login_banner())

	for {
		if !c.connected {
			return
		}

		time.Sleep(time.Duration(25) * time.Millisecond)
		if !c.loggedin {
			c.send("Auth: ")
			in := c.recv()
			switch {
			case s.authCheck(c, in):
				c.loggedin = true
				c.deadline = time.Duration(480) * time.Second
				c.send("successful login")
				continue
			case in == "register":
				if !RegRater.Check(c) {
					println("new registration from " + c.UniqueKey())
					s.setID(c, s.getUnusedID())
					c.send("\nregistration success\n[New ID]: " + c.ID)
					return
				} else {
					c.send("you already registered recently\n")
				}
				continue
			default:
				c.send("invalid. type 'REGISTER' to register a new ID\n")
				continue
			}
		}

		c.send("\nRate5 > ")
		switch c.recv() {
		case "history":
			c.send("account logins:\n")
			for _, login := range s.AuthLog[c.ID] {
				c.send(login.Time.Format("Mon, 02 Jan 2006 15:04:05 MST") + ": " + login.IP + "\n")
			}
		case "help":
			c.send("history, whoami, logout\n")
		case "whoami":
			c.send(c.ID + "\n")
		case "quit":
			fallthrough
		case "exit":
			fallthrough
		case "logout":
			c.loggedin = false
			return
		}
	}
}

func (c *Client) send(data string) {
	c.Conn.SetReadDeadline(time.Now().Add(c.deadline))
	if _, err := c.Conn.Write([]byte(data)); err != nil {
		c.connected = false
	}
}

func (c *Client) recv() string {
	c.Conn.SetReadDeadline(time.Now().Add(c.deadline))
	if CmdRater.Check(c) {
		if !c.loggedin {
			// if they hit the ratelimiter during log-in, disconnect them
			c.connected = false
		}
		time.Sleep(time.Duration(1250) * time.Millisecond)
	}
	in, err := c.read.ReadString('\n')
	if err != nil {
		println(c.UniqueKey() + ": " + err.Error())
		c.connected = false
		return in
	}
	c.read.Reset(c.Conn)
	return strings.ToLower(strings.TrimRight(in, "\n"))
}

const charset = "abcdefghijklmnopqrstuvwxyz1234567890"

var rngseed *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func keygen() string {
	b := make([]byte, keySize)
	for i := range b {
		b[i] = charset[rngseed.Intn(len(charset))]
	}
	return string(b)
}

// getUnusedKey assures that our newly generated ID is not in use
func (s *Server) getUnusedID() string {
	s.mu.RLock()
	var newkey string
	for {
		newkey = keygen()
		if _, ok := s.Map[newkey]; !ok {
			break
		} else {
			println("key already exists! generating new...")
		}
	}
	s.mu.RUnlock()
	return newkey
}

// setID sets the clients ID safely
func (s *Server) setID(c *Client, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = id
	s.Map[id] = c
}

func (s *Server) replaceSession(c *Client, id string) {
	s.mu.Lock()
	s.AuthLog[id] = append(s.AuthLog[id], Login{
		// we're not logged in so UniqueKey is still the IP address
		IP:   c.UniqueKey(),
		Time: time.Now(),
	})
	defer s.mu.Unlock()
	delete(s.Map, id)
	s.Map[id] = c
	c.ID = id
}

func (s *Server) authCheck(c *Client, id string) bool {
	s.mu.RLock()
	if old, ok := s.Map[id]; ok {
		s.mu.RUnlock()
		old.connected = false
		old.Conn.Close()
		s.replaceSession(c, id)
		return true
	}

	s.mu.RUnlock()
	return false

}

func login_banner() []byte {
	login := "CnwgG1s5MDs0MG1SG1swbRtbMG0gG1s5Nzs0MG3DhhtbMG0bWzBtIBtbOTc7NDBtzpMbWzBtG1swbSAbWzk3OzQwbc6jG1swbRtbMG0gG1swbRtbOTc7MzJtNRtbMG0bWzBtIHwKCg=="
	str, _ := base64.StdEncoding.DecodeString(login)
	return str
}

func init() {
	// Rater is our connection ratelimiter
	Rater = rate5.NewDefaultLimiter()
	// RegRater will only allow one registration per 50 seconds and will add to the wait each time you get limited
	RegRater = rate5.NewStrictLimiter(50, 1)
	// CmdRater will slow down commands send when connected
	CmdRater = rate5.NewLimiter(10, 20)

	srv = &Server{
		Map:     make(map[string]*Client),
		AuthLog: make(map[string][]Login),
		mu:      &sync.RWMutex{},
	}

	rd := Rater.DebugChannel()
	rrd := RegRater.DebugChannel()
	crd := CmdRater.DebugChannel()

	pre := "[Rate5] "
	go func() {
		for {
			select {
			case msg := <-rd:
				println(pre + "Limit: " + msg)
			case msg := <-rrd:
				println(pre + "RegLimit: " + msg)
			case msg := <-crd:
				println(pre + "CmdLimit: " + msg)
			default:
				time.Sleep(time.Duration(10) * time.Millisecond)
			}
		}
	}()
}

func main() {
	l, err := net.Listen("tcp", "127.0.0.1:4444")
	if err != nil {
		panic(err.Error())
	}
	println("listening...")

	for {
		conn, err := l.Accept()
		if err != nil {
			println(err.Error())
		}
		go srv.handleTCP(&Client{
			Conn:      conn,
			connected: true,
			loggedin:  false,
			deadline:  time.Duration(12) * time.Second,
		})
	}
}
