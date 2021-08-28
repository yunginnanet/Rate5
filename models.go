package ratelimit

import (
	cache "github.com/patrickmn/go-cache"
	"sync"
)

const (
	// DefaultWindow is the standard window of time ratelimit triggers are observed in seconds
	DefaultWindow = 25
	// DefaultWindow is the standard amount of triggers observed within Window before ratelimiting occurs
	DefaultBurst = 25
)

var debugChannel chan string

/* any type implementing Identity only needs to have one unique key for the ratelimiter */
type Identity interface {
	UniqueKey() string
}

// Limiter implements an Enforcer to create an arbitrary ratelimiter
type Limiter struct {
	Source Identity
	// Patrons are the IRC users that we are rate limiting
	Patrons *cache.Cache
	// Ruleset is the actual ratelimitting model
	Ruleset Policy
	/* Debug mode (toggled here) enables debug messages
	   delivered through a channel. See: DebugChannel() */
	Debug bool

	known map[interface{}]int
	mu *sync.Mutex
}

// Policy defines the mechanics of our ratelimiter
type Policy struct {
	// Window defines the duration in seconds that we should keep track of ratelimit triggers
	Window int
	/* Burst is the amount of times that Check will not trigger a limit
	   within the duration defined by Window */
	Burst int
	/* Strict mode punishes triggers of the ratelimit
	   by increasing the amount of time they have to wait
	   every time they trigger the limitter */
	Strict bool
}
