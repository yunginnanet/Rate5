package rate5

import (
	"fmt"
	"sync"

	"github.com/patrickmn/go-cache"
)

const (
	// DefaultWindow is the standard window of time ratelimit triggers are observed in seconds.
	DefaultWindow = 25
	// DefaultBurst is the standard amount of triggers observed within Window before ratelimiting occurs.
	DefaultBurst = 25
)

// Identity is an interface that allows any arbitrary type to be used for a unique key in ratelimit checks when implemented.
type Identity interface {
	UniqueKey() string
}

// IdentityStringer is an implentation of Identity that acts as a shim for types that implement fmt.Stringer.
type IdentityStringer struct {
	stringer fmt.Stringer
}

func (i IdentityStringer) UniqueKey() string {
	return i.stringer.String()
}

const (
	DebugDisabled uint32 = iota
	DebugEnabled
)

// Limiter implements an Enforcer to create an arbitrary ratelimiter.
type Limiter struct {
	// Patrons gives access to the underlying cache type that powers the ratelimiter.
	// It is exposed for testing purposes.
	Patrons *cache.Cache

	// Ruleset determines the Policy which is used to determine whether or not to ratelimit.
	// It consists of a Window and Burst, see Policy for more details.
	Ruleset Policy

	debug        uint32
	debugChannel chan string
	debugLost    int64
	known        map[interface{}]*int64
	debugMutex   *sync.RWMutex
	*sync.RWMutex
}

// Policy defines the mechanics of our ratelimiter.
type Policy struct {
	// Window defines the duration in seconds that we should keep track of ratelimit triggers,
	Window int64
	// Burst is the amount of times that Check will not trigger a limit within the duration defined by Window.
	Burst int64
	// Strict mode punishes triggers of the ratelimitter by increasing the wait time upon every trigger of the limiter.
	Strict bool
	// Hardcore mode implies strict mode but instead of using addition when adding to the wait time, it uses multiplication.
	// This will cause exponential ratelimiting.
	Hardcore bool
}
