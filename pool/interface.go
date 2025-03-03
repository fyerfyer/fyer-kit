package pool

import (
	"context"
	"io"
	"time"
)

// Connection represents a managed connection from a pool.
// It extends io.Closer to ensure connections can be properly closed.
type Connection interface {
	io.Closer

	// Raw returns the underlying connection object.
	// The returned value can be type asserted to the actual connection type.
	Raw() interface{}

	// IsAlive checks if the connection is still valid and usable.
	IsAlive() bool

	// ResetState prepares the connection for reuse.
	ResetState() error
}

// ConnectionFactory creates new connections for a pool.
type ConnectionFactory interface {
	// Create creates a new connection.
	Create(ctx context.Context) (Connection, error)
}

// Pool manages a collection of reusable connections.
type Pool interface {
	// Get retrieves a connection from the pool or creates a new one if needed.
	// The context can be used to control timeouts or cancellation.
	Get(ctx context.Context) (Connection, error)

	// Put returns a connection to the pool or closes it if the pool is full.
	// If the connection is not usable (err != nil), it will be closed instead of returned to the pool.
	Put(conn Connection, err error) error

	// Shutdown gracefully shuts down the pool, waiting for connections to be returned.
	// It will wait until the context is canceled or all connections are returned.
	Shutdown(ctx context.Context) error

	// Stats returns statistics about the pool's current state.
	Stats() Stats
}

// Stats represents pool statistics.
type Stats struct {
	// Active is the number of connections currently checked out from the pool
	Active int

	// Idle is the number of connections currently idle in the pool
	Idle int

	// Total is the total number of connections in the pool (Active + Idle)
	Total int

	// Waiters is the number of callers waiting for a connection
	Waiters int

	// Timeouts is the number of Get operations that timed out
	Timeouts int64

	// Errors is the number of failed connection attempts
	Errors int64

	// Acquired is the total number of successful Get operations
	Acquired int64

	// Released is the total number of successful Put operations
	Released int64

	// MaxIdleTime is the maximum time a connection can remain idle before being closed
	MaxIdleTime time.Duration

	// MaxLifetime is the maximum time a connection can live since creation
	MaxLifetime time.Duration

	// CreatedAt is when the pool was created
	CreatedAt time.Time
}

// PoolManager manages multiple connection pools.
type PoolManager interface {
	// Get retrieves a named connection pool.
	Get(name string) (Pool, error)

	// Register registers a new connection pool with the given name.
	Register(name string, pool Pool) error

	// Remove removes a named connection pool.
	Remove(name string) error

	// Shutdown gracefully shuts down all pools.
	Shutdown(ctx context.Context) error

	// Stats returns statistics for all pools.
	Stats() map[string]Stats
}

// State represents the connection state.
type State int

const (
	// StateIdle indicates the connection is in the pool and not being used.
	StateIdle State = iota

	// StateInUse indicates the connection is currently being used.
	StateInUse

	// StateClosed indicates the connection has been closed.
	StateClosed
)

// Event represents a connection lifecycle event.
type Event int

const (
	// EventGet is triggered when a connection is retrieved from the pool.
	EventGet Event = iota

	// EventPut is triggered when a connection is returned to the pool.
	EventPut

	// EventClose is triggered when a connection is closed.
	EventClose

	// EventNew is triggered when a new connection is created.
	EventNew
)

// EventListener is notified about connection lifecycle events.
type EventListener interface {
	// OnEvent is called when a connection event occurs.
	OnEvent(event Event, conn Connection)
}
