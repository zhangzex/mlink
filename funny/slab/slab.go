package slab
/*slab算法的内存池*/
type Pool interface {
	Alloc(int) []byte // 分配字节数组
	Free([]byte) //空闲的字节数组
}

type NoPool struct{}

func (p *NoPool) Alloc(size int) []byte {
	return make([]byte, size) // 创建指定大小的分片
}

func (p *NoPool) Free(_ []byte) {}

var _ Pool = (*NoPool)(nil)
var _ Pool = (*ChanPool)(nil)
var _ Pool = (*SyncPool)(nil)
var _ Pool = (*AtomPool)(nil)
