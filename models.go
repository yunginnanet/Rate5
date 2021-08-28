package ratelimit

import (
	cache "github.com/patrickmn/go-cache"
	"time"
)

const (
	DefaultWindow     = 10
	DefaultBurst      = 30
	DefaultStrictMode = true
)

var debugChannel chan string

/* any type implementing Identity only needs to have one unique key for the ratelimiter */
type Identity interface {
	UniqueKey() string
}

// Queue implements an Enforcer to create an arbitrary ratelimiter
type Queue struct {
	Source Identity
	// Patrons are the IRC users that we are rate limiting
	Patrons *cache.Cache
	// Ruleset is the actual ratelimitting model
	Ruleset Policy
	Known   map[interface{}]time.Duration
	Debug   bool
}

// Policy defines the mechanics of our ratelimiter
type Policy struct {
	// Window defines the seconds between each post from an IP address
	Window time.Duration
	// Burst is used differently based on Strict mode
	Burst int
	// Strict TODO: document this
	Strict bool
}
