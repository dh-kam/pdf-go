package xpath

import "sync"

// xpathPool reuses transient XPath buffers across rasterisation calls (SP2 design §3).
var xpathPool = sync.Pool{
	New: func() interface{} { return &XPath{} },
}

var scannerPool = sync.Pool{
	New: func() interface{} { return &Scanner{} },
}

var pathPool = sync.Pool{
	New: func() interface{} { return &Path{} },
}

var clipPool = sync.Pool{
	New: func() interface{} { return &Clip{} },
}

// TrimMemoryPools drops released XPath, scanner, path, and clip buffers
// retained for reuse. Callers must not invoke it concurrently with rendering.
func TrimMemoryPools() {
	xpathPool = sync.Pool{New: func() interface{} { return &XPath{} }}
	scannerPool = sync.Pool{New: func() interface{} { return &Scanner{} }}
	pathPool = sync.Pool{New: func() interface{} { return &Path{} }}
	clipPool = sync.Pool{New: func() interface{} { return &Clip{} }}
}

const maxPooledXPathCapacity = 1 << 17
const maxPooledScannerCapacity = 1 << 17

// AcquireXPath fetches a reset XPath from the transient rasterisation pool.
func AcquireXPath() *XPath {
	return acquireXPath()
}

// acquireXPath fetches a reset XPath from the pool.
func acquireXPath() *XPath {
	x := xpathPool.Get().(*XPath)
	x.Segs = x.Segs[:0]
	x.pts = x.pts[:0]
	return x
}

// ReleaseXPath returns x to the transient rasterisation pool.
func ReleaseXPath(x *XPath) {
	releaseXPath(x)
}

// AcquireScanner builds a transient scanner backed by pooled row buffers.
func AcquireScanner(x *XPath, eo bool, xMinA, yMinA, xMaxA, yMaxA int) *Scanner {
	return acquireScanner(x, eo, xMinA, yMinA, xMaxA, yMaxA, 0)
}

func acquireScanner(x *XPath, eo bool, xMinA, yMinA, xMaxA, yMaxA int, yFloorSnapEps float64) *Scanner {
	s := scannerPool.Get().(*Scanner)
	s.reset(x, eo, xMinA, yMinA, xMaxA, yMaxA, yFloorSnapEps)
	return s
}

// ReleaseScanner returns a transient scanner and its buffers to the pool.
func ReleaseScanner(s *Scanner) {
	releaseScanner(s)
}

// AcquirePathWithCapacity fetches a transient path with enough point storage.
func AcquirePathWithCapacity(pointCapacity int) *Path {
	return AcquirePathWithCapacities(pointCapacity, 0)
}

// AcquirePathWithCapacities fetches a transient path with enough point and hint storage.
func AcquirePathWithCapacities(pointCapacity, hintCapacity int) *Path {
	p := pathPool.Get().(*Path)
	p.Reset()
	if pointCapacity > 0 && cap(p.pts) < pointCapacity {
		p.pts = make([]PathPoint, 0, pointCapacity)
		p.flags = make([]byte, 0, pointCapacity)
	}
	if hintCapacity > 0 && cap(p.hints) < hintCapacity {
		p.hints = make([]PathHint, 0, hintCapacity)
	}
	return p
}

// ReleasePath returns a transient path to the pool.
func ReleasePath(p *Path) {
	if p == nil {
		return
	}
	p.Reset()
	pathPool.Put(p)
}

func acquireClip() *Clip {
	return clipPool.Get().(*Clip)
}

// ReleaseClip returns an owned scannerless clip to the pool.
//
// Path clips carry shared scanner pointers and do not currently have reference
// counts, so they are intentionally left for the garbage collector.
func ReleaseClip(c *Clip) {
	if c == nil || len(c.scanners) > 0 {
		return
	}
	*c = Clip{}
	clipPool.Put(c)
}

// ReleaseOwnedClip returns an exclusively owned clip to the pool.
//
// Scanner pointers at indexes below sharedScannerCount may be shared with a
// cloned clip, so only owned tail scanners are returned to the scanner pool.
func ReleaseOwnedClip(c *Clip) {
	if c == nil {
		return
	}
	c.releaseOwnedScanners()
	*c = Clip{}
	clipPool.Put(c)
}

// releaseXPath returns x to the pool after truncating its segment slice for reuse.
func releaseXPath(x *XPath) {
	if x == nil {
		return
	}
	if cap(x.Segs) > maxPooledXPathCapacity {
		x.Segs = nil
	} else {
		x.Segs = x.Segs[:0]
	}
	if cap(x.pts) > maxPooledXPathCapacity {
		x.pts = nil
	} else {
		x.pts = x.pts[:0]
	}
	if cap(x.adjusts) > maxPooledXPathCapacity {
		x.adjusts = nil
	} else {
		x.adjusts = x.adjusts[:0]
	}
	xpathPool.Put(x)
}

func releaseScanner(s *Scanner) {
	if s == nil {
		return
	}
	s.xPath = nil
	if len(s.allIntersections) > 0 {
		clear(s.allIntersections)
	}
	if cap(s.countScratch) > maxPooledScannerCapacity {
		s.countScratch = nil
	} else {
		s.countScratch = s.countScratch[:0]
	}
	if cap(s.allIntersections) > maxPooledScannerCapacity {
		s.allIntersections = nil
	} else {
		s.allIntersections = s.allIntersections[:0]
	}
	if cap(s.intersections) > maxPooledScannerCapacity {
		s.intersections = nil
	} else {
		s.intersections = s.intersections[:0]
	}
	scannerPool.Put(s)
}

func (c *Clip) releaseOwnedScanners() {
	if c == nil {
		return
	}
	start := c.sharedScannerCount
	if start < 0 {
		start = 0
	}
	if start > len(c.scanners) {
		start = len(c.scanners)
	}
	for _, scanner := range c.scanners[start:] {
		releaseScanner(scanner)
	}
	c.scanners = nil
	c.sharedScannerCount = 0
}
