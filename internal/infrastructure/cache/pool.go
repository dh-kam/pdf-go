// Package cache provides object pool implementations.
package cache

import (
	"sync"

	domaincache "github.com/dh-kam/pdf-go/internal/domain/cache"
)

// bytePool implements a pool of byte slices.
type bytePool struct {
	pool        sync.Pool
	minSize     int
	maxSize     int
	currentSize int
	mu          sync.Mutex
}

// NewBytePool creates a new byte pool.
func NewBytePool(config domaincache.PoolConfig) domaincache.BytePool {
	if config.MinSize <= 0 {
		config.MinSize = 10
	}
	if config.MaxSize <= 0 {
		config.MaxSize = 1000
	}

	p := &bytePool{
		minSize:     config.MinSize,
		maxSize:     config.MaxSize,
		currentSize: 0,
	}

	// Set up the pool with a New function
	p.pool.New = func() interface{} {
		if config.NewFunc != nil {
			return config.NewFunc()
		}
		// Default: create a 4KB buffer
		return make([]byte, 4096)
	}

	return p
}

// Get retrieves a byte slice from the pool.
func (p *bytePool) Get() interface{} {
	return p.pool.Get()
}

// Put returns a byte slice to the pool.
func (p *bytePool) Put(item interface{}) {
	p.pool.Put(item)
}

// New creates a new byte slice.
func (p *bytePool) New() interface{} {
	return make([]byte, 4096)
}

// Size returns the number of objects in the pool.
func (p *bytePool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentSize
}

// GetBytes retrieves a byte slice of the given size.
func (p *bytePool) GetBytes(size int) []byte {
	buf := p.pool.Get()
	if b, ok := buf.([]byte); ok {
		if cap(b) >= size {
			return b[:size]
		}
		// Buffer too small, discard and create new one
		return make([]byte, size)
	}
	if b, ok := buf.(*[]byte); ok && b != nil {
		if cap(*b) >= size {
			return (*b)[:size]
		}
		return make([]byte, size)
	}
	// Not a byte slice, create new one
	return make([]byte, size)
}

// PutBytes returns a byte slice to the pool.
func (p *bytePool) PutBytes(buf []byte) {
	if cap(buf) < 4096 || cap(buf) > 1024*1024 {
		// Only pool buffers in reasonable size range
		return
	}
	p.pool.Put(&buf)
}

// bufferPool implements a sync.Pool wrapper for buffer buffers.
type bufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new buffer pool.
func NewBufferPool() domaincache.Pool {
	return &bufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 4096)
			},
		},
	}
}

// Get retrieves an object from the pool.
func (p *bufferPool) Get() interface{} {
	return p.pool.Get()
}

// Put returns an object to the pool.
func (p *bufferPool) Put(item interface{}) {
	p.pool.Put(item)
}

// New creates a new object.
func (p *bufferPool) New() interface{} {
	return make([]byte, 0, 4096)
}

// Size returns the number of objects in the pool.
func (p *bufferPool) Size() int {
	// sync.Pool doesn't expose size
	return -1
}
