package slab

import "unsafe"

// ChanPool is a chan based slab allocation memory pool. > ChanPool是基于chan的板分配内存池。
type ChanPool struct {
	classes []chanClass
	minSize int
	maxSize int
}

// NewChanPool create a chan based slab allocation memory pool. > NewChanPool是基于chan基础创建的板分配内存池。
// minSize is the smallest chunk size. > minSize是最小的块。
// maxSize is the lagest chunk size. > maxSize 是最大的块。
// factor is used to control growth of chunk size. > factor用于控制块大小的增长。
// pageSize is the memory size of each slab class. > pageSize是每个slab类的内存大小。
func NewChanPool(minSize, maxSize, factor, pageSize int) *ChanPool {
	pool := &ChanPool{make([]chanClass, 0, 10), minSize, maxSize}
	for chunkSize := minSize; chunkSize <= maxSize && chunkSize <= pageSize; chunkSize *= factor {
		c := chanClass{
			size:   chunkSize,
			page:   make([]byte, pageSize),
			chunks: make(chan []byte, pageSize/chunkSize),
		}
		c.pageBegin = uintptr(unsafe.Pointer(&c.page[0]))
		for i := 0; i < pageSize/chunkSize; i++ {
			// lock down the capacity to protect append operation
			mem := c.page[i*chunkSize : (i+1)*chunkSize : (i+1)*chunkSize]
			c.chunks <- mem
			if i == len(c.chunks)-1 {
				c.pageEnd = uintptr(unsafe.Pointer(&mem[0]))
			}
		}
		pool.classes = append(pool.classes, c)
	}
	return pool
}
// > alloc尝试从内部slab类分配一个字节切片[]byte，如果slab类中没有空闲块，alloc将生成一个。 (如果slab类中没有空闲块,alloc尝试从内部slab类分配一个字节切片[]byte )
// Alloc try alloc a []byte from internal slab class if no free chunk in slab class Alloc will make one.
func (pool *ChanPool) Alloc(size int) []byte {
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

// Free release a []byte that alloc from Pool.Alloc.  >释放pool.alloc中alloc的一个字节切片[]byte。
func (pool *ChanPool) Free(mem []byte) {
	size := cap(mem)
	for i := 0; i < len(pool.classes); i++ {
		if pool.classes[i].size == size {
			pool.classes[i].Push(mem)
			break
		}
	}
}
// >创建chanClass结构体
type chanClass struct { 
	size      int
	page      []byte
	pageBegin uintptr
	pageEnd   uintptr
	chunks    chan []byte
}
// > chanClass结构体的 push[推] 方法，需要传入字节切片作为参数。
func (c *chanClass) Push(mem []byte) {
	select {
	case c.chunks <- mem:
	default:
		mem = nil
	}
}
// > chanClass结构体的Pop方法 ,返回字节数组
func (c *chanClass) Pop() []byte {
	select {
	case mem := <-c.chunks:
		return mem
	default:
		return nil
	}
}
