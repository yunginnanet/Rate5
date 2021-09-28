package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	rate5 "github.com/yunginnanet/Rate5"
)

// characters used for registration IDs
const charset = "abcdefghijklmnopqrstuvwxyz1234567890"

var (
	// Rater is our connection ratelimiter using default limiter settings.
	Rater *rate5.Limiter
	// RegRater will only allow one registration per 50 seconds and will add to the wait each time you get limited. (by IP)
	RegRater *rate5.Limiter
	// CmdRater will slow down commands sent, if not logged in by IP, if logged in by ID.
	CmdRater *rate5.Limiter

	srv     *Server
	keySize = 8
)

// Server is an instance of our concurrent TCP server including a map of active clients
type Server struct {
	Map     map[string]*Client
	AuthLog map[string][]Login
	Exempt  map[string]bool
	mu      *sync.RWMutex
}

// Login represents a successful login by a user
type Login struct {
	IP   string
	Time time.Time
}

// Client represents a known patron of our Server
type Client struct {
	ID   string
	Conn net.Conn

	loggedin  bool
	connected bool
	authlog   []Login

	deadline time.Duration
	read     *bufio.Reader
}

// UniqueKey is an implementation of our Identity interface, in short: Rate5 doesn't care where you derive the string used for ratelimiting
func (c Client) UniqueKey() string {
	var err error
	var host string
	if c.loggedin {
		return c.ID
	}
	if host, _, err = net.SplitHostPort(c.Conn.RemoteAddr().String()); err == nil {
		return host
	}
	panic(err)
}

func argParse() {
	if len(os.Args) < 1 {
		return
	}
	for i, arg := range os.Args {
		switch arg {
		case "-e":
			fallthrough
		case "--exempt":
			if len(os.Args) <= i+1 {
				return
			}
			srv.Exempt[os.Args[i+1]] = true
		default:
			continue
		}

	}
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
		Exempt:  make(map[string]bool),

		mu: &sync.RWMutex{},
	}

	argParse()

	rd := Rater.DebugChannel()
	rrd := RegRater.DebugChannel()
	crd := CmdRater.DebugChannel()
	go watchDebug(rd, rrd, crd)
}

func watchDebug(rd, rrd, crd chan string) {
	pre := "[Rate5] "
	var lastcount = 0
	var count = 0
	for {
		select {
		case msg := <-rd:
			fmt.Printf("%s Limit: %s \n", pre, msg)
			count++
		case msg := <-rrd:
			fmt.Printf("%s RegLimit: %s \n", pre, msg)
			count++
		case msg := <-crd:
			fmt.Printf("%s CmdLimit: %s \n", pre, msg)
			count++
		default:
			if count-lastcount >= 25 {
				lastcount = count
				fmt.Println("Rater: ", Rater.GetGrandTotalRated())
				fmt.Println("RegRater: ", RegRater.GetGrandTotalRated())
				fmt.Println("CmdRater: ", CmdRater.GetGrandTotalRated())
			}
			time.Sleep(time.Duration(10) * time.Millisecond)
		}
	}
}

func (s *Server) preLogin(c *Client) {
	c.send("Auth: ")
	in := c.recv()
	switch {
	case s.authCheck(c, in):
		c.loggedin = true
		c.deadline = time.Duration(480) * time.Second
		c.send("successful login")
		return
	case in == "register":
		// no exemption for strict ratelimiter (rate5 testing)
		if RegRater.Check(c) {
			c.send("you already registered recently\n")
			return
		}
		println("new registration from " + c.UniqueKey())
		s.setID(c, s.getUnusedID())
		c.send("\nregistration success\n[New ID]: " + c.ID)
		return
	default:
		c.send("invalid. type 'REGISTER' to register a new ID\n")
		return
	}

}

func (s *Server) mainPrompt(c *Client) {
	for c.connected {
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
		case "logout":
			c.loggedin = false
			return
		default:
			c.send("unknown command, are you lost?")
			continue
		}
	}
}

func isExempt(c *Client) bool {
	srv.mu.RLock()
	_, exempt := srv.Exempt[c.UniqueKey()]
	srv.mu.RUnlock()
	return exempt
}

func connRateCheck(c *Client) bool {
	if isExempt(c) {
		return false
	}
	if Rater.Check(c) {
		c.send("too many connections")
		println(c.UniqueKey() + " ratelimited")
		return true
	}
	return false
}

func closeConn(c *Client) {
	if err := c.Conn.Close(); err != nil {
		println(err.Error())
	}
	println("closed: " + c.Conn.RemoteAddr().String())
}

func (s *Server) handleTCP(c *Client) {
	if err := c.Conn.(*net.TCPConn).SetLinger(0); err != nil {
		fmt.Println("error while setting setlinger:", err.Error())
	}
	defer closeConn(c)
	if rated := connRateCheck(c); rated {
		return
	}
	c.read = bufio.NewReader(c.Conn)
	if _, err := c.Conn.Write(loginBanner()); err != nil {
		return
	}
	for !c.loggedin {
		if !c.connected {
			return
		}
		s.preLogin(c)
	}
	s.mainPrompt(c)
}

func (c *Client) send(data string) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.deadline)); err != nil {
		fmt.Println("error while setting deadline:", err.Error())
	}
	if _, err := c.Conn.Write([]byte(data)); err != nil {
		c.connected = false
	}
}

func (c *Client) recv() string {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.deadline)); err != nil {
		fmt.Println("error while setting deadline:", err.Error())
	}

	if !isExempt(c) {
		if CmdRater.Check(c) {
			if !c.loggedin {
				// if they hit the ratelimiter during log-in, disconnect them
				c.connected = false
			}
			time.Sleep(time.Duration(1250) * time.Millisecond)
		}
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

func randUint32() uint32 {
	b := make([]byte, 4096)
	if _, err := rand.Read(b); err != nil {

		panic(err)
	}
	return binary.BigEndian.Uint32(b)
}

func keygen() string {
	chrlen := len(charset)
	b := make([]byte, chrlen)
	for i := 0; i != keySize; i++ {
		b[i] = charset[randUint32()%uint32(chrlen)]
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
		closeConn(old)
		s.replaceSession(c, id)
		return true
	}

	s.mu.RUnlock()
	return false
}

func loginBanner() []byte {
	var data []byte
	var err error
	login := "CnwgG1s5MDs0MG1SG1swbRtbMG0gG1s5Nzs0MG3DhhtbMG0bWzBtIBtbOTc7NDBtzpMbWzBtG1swbSAbWzk3OzQwbc6jG1swbRtbMG0gG1swbRtbOTc7MzJtNRtbMG0bWzBtIHwKCg=="
	if data, err = base64.StdEncoding.DecodeString(login); err == nil {
		return data
	}
	panic(err)
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
