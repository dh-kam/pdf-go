package xpath

import "sync"

// xpathPool reuses transient XPath buffers across rasterisation calls (SP2 design §3).
var xpathPool = sync.Pool{
	New: func() interface{} { return &XPath{} },
}

// acquireXPath fetches a reset XPath from the pool.
func acquireXPath() *XPath {
	x := xpathPool.Get().(*XPath)
	x.Segs = x.Segs[:0]
	return x
}

// releaseXPath returns x to the pool after truncating its segment slice for reuse.
func releaseXPath(x *XPath) {
	if x == nil {
		return
	}
	x.Segs = x.Segs[:0]
	xpathPool.Put(x)
}
