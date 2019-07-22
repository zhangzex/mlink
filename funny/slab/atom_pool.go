package slab

import (
	"reflect"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// AtomPool is a lock-free slab allocation memory pool. >atompool是一个无锁的板分配内存池。
type AtomPool struct {
	classes []class
	minSize int
	maxSize int
}

// NewAtomPool create a lock-free slab allocation memory pool. >newatompool创建一个无锁的slab分配内存池。 
// minSize is the smallest chunk size. >minSize是最小的块。
// maxSize is the lagest chunk size. >maxSize是最大的块。
// factor is used to control growth of chunk size. >factor用于控制块大小的增长
// pageSize is the memory size of each slab class. >pageSize是每个slab类的内存大小。 
func NewAtomPool(minSize, maxSize, factor, pageSize int) *AtomPool {
	pool := &AtomPool{make([]class, 0, 10), minSize, maxSize}
	for chunkSize := minSize; chunkSize <= maxSize && chunkSize <= pageSize; chunkSize *= factor {
		c := class{
			size:   chunkSize,
			page:   make([]byte, pageSize),
			chunks: make([]chunk, pageSize/chunkSize),
			head:   (1 << 32),
		}
		for i := 0; i < len(c.chunks); i++ {
			chk := &c.chunks[i]
			// lock down the capacity to protect append operation >锁定保护附加操作的能力
			chk.mem = c.page[i*chunkSize : (i+1)*chunkSize : (i+1)*chunkSize]
			if i < len(c.chunks)-1 {
				chk.next = uint64(i+1+1 /* index start from 1 */) << 32
			} else {
				c.pageBegin = uintptr(unsafe.Pointer(&c.page[0]))
				c.pageEnd = uintptr(unsafe.Pointer(&chk.mem[0]))
			}
		}
		pool.classes = append(pool.classes, c)
	}
	return pool
}
// >alloc尝试从内部slab类分配一个[]字节，如果slab类中没有空闲块，alloc将生成一个。
// Alloc try alloc a []byte from internal slab class if no free chunk in slab class Alloc will make one.
func (pool *AtomPool) Alloc(size int) []byte {
	if size <= pool.maxSize {
		for i := 0; i < len(pool.classes); i++ {
			if pool.classes[i].size >= size {
				mem := pool.classes[i].Pop()
				if mem != nil {
					return mem[:size]
				}
				break
			}
		}
	}
	return make([]byte, size)
}
// >释放pool.alloc中alloc的一个[]字节。 
// Free release a []byte that alloc from Pool.Alloc.
func (pool *AtomPool) Free(mem []byte) {
	size := cap(mem)
	for i := 0; i < len(pool.classes); i++ {
		if pool.classes[i].size == size {
			pool.classes[i].Push(mem)
			break
		}
	}
}

type class struct {
	size      int
	page      []byte
	pageBegin uintptr
	pageEnd   uintptr
	chunks    []chunk
	head      uint64
}

type chunk struct {
	mem  []byte
	aba  uint32 // reslove ABA problem
	next uint64
}

func (c *class) Push(mem []byte) {
	ptr := (*reflect.SliceHeader)(unsafe.Pointer(&mem)).Data
	if c.pageBegin <= ptr && ptr <= c.pageEnd {
		i := (ptr - c.pageBegin) / uintptr(c.size)
		chk := &c.chunks[i]
		if chk.next != 0 {
			panic("slab.AtomPool: Double Free")
		}
		chk.aba++
		new := uint64(i+1)<<32 + uint64(chk.aba)
		for {
			old := atomic.LoadUint64(&c.head)
			atomic.StoreUint64(&chk.next, old)
			if atomic.CompareAndSwapUint64(&c.head, old, new) {
				break
			}
			runtime.Gosched()
		}
	}
}

func (c *class) Pop() []byte {
	for {
		old := atomic.LoadUint64(&c.head)
		if old == 0 {
			return nil
		}
		chk := &c.chunks[old>>32-1]
		nxt := atomic.LoadUint64(&chk.next)
		if atomic.CompareAndSwapUint64(&c.head, old, nxt) {
			atomic.StoreUint64(&chk.next, 0)
			return chk.mem
		}
		runtime.Gosched()
	}
}
