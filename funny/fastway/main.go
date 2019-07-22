package main

import (
	"flag"
	"log"
	"net"
	"time"

	_ "github.com/funny/cmd"
	fastway "github.com/funny/fastway/go"
	"github.com/funny/reuseport"
	"github.com/funny/slab"
	snet "github.com/funny/snet/go"
)

var (  
	//网关相关参数
	reusePort       = flag.Bool("ReusePort", false, "是否开启reuseport特性，默认false")
	maxPacket       = flag.Int("MaxPacket", 512*1024, "最大的消息包体积，默认512k。")
	memPoolType     = flag.String("MemPoolType", "atom", "slab内存池类型 (sync、atom或chan)，默认atom")
	memPoolFactor   = flag.Int("MemPoolFactor", 2, "slab内存池的Chunk递增指数，默认是 2")
	memPoolMinChunk = flag.Int("MemPoolMinChunk", 64, "slab内存池中最小的Chunk大小，默认是64B。")
	memPoolMaxChunk = flag.Int("MemPoolMaxChunk", 64*1024, "slab内存池中最大的Chunk大小，默认64K")
	memPoolPageSize = flag.Int("MemPoolPageSize", 1024*1024, "slab内存池的每个Slab内存大小，默认是 1M")
	
	//客户端相关参数
	clientAddr            = flag.String("ClientAddr", ":0", "网关暴露给客户端的地址")
	clientMaxConn         = flag.Int("ClientMaxConn", 16, "每个客户端可以创建的最大虚拟连接数")
	clientBufferSize      = flag.Int("ClientBufferSize", 2*1024, "每个客户端连接使用的 bufio.Reader 缓冲区大小")
	clientSendChanSize    = flag.Int("ClientSendChanSize", 1024, "每个客户端连接异步发送消息用的chan缓冲区大小")
	clientIdleTimeout     = flag.Duration("ClientIdleTimeout", 30*time.Second, "客户端连接空闲超过时长，客户端应在此时间范围内发送ping命令来保活")
	clientSnetEnable      = flag.Bool("ClientSnetEnable", false, "是否为客户端开启snet协议")
	clientSnetEncrypt     = flag.Bool("ClientSnetEncrypt", false, "是否为客户端开启snet加密")
	clientSnetBuffer      = flag.Int("ClientSnetBuffer", 64*1024, "每个客户端物理连接对应的snet重连缓冲区大小")
	clientSnetInitTimeout = flag.Duration("ClientSnetInitTimeout", 10*time.Second, "客户端snet握手超时时间。")
	clientSnetWaitTimeout = flag.Duration("ClientSnetWaitTimeout", 60*time.Second, "客户端snet等待重连超时时间")

	//服务端相关参数
	serverAddr            = flag.String("ServerAddr", ":0", "网关暴露给服务端的地址")
	serverAuthKey         = flag.String("ServerAuthKey", "", "用于验证服务端合法性的秘钥")
	serverBufferSize      = flag.Int("ServerBufferSize", 64*1024, "每个服务端连接使用的 bufio.Reader 缓冲区大小")
	serverSendChanSize    = flag.Int("ServerSendChanSize", 102400, "每个服务端连接异步发送消息用的chan缓冲区大小。")
	serverIdleTimeout     = flag.Duration("ServerIdleTimeout", 30*time.Second, "服务端连接空闲超过时长，服务端应在此时间范围内发送ping命令来保活")
	serverSnetEnable      = flag.Bool("ServerSnetEnable", false, "是否为服务端开启snet协议。")
	serverSnetEncrypt     = flag.Bool("ServerSnetEncrypt", false, ">启用/禁用服务端开启snet加密。")
	serverSnetBuffer      = flag.Int("ServerSnetBuffer", 64*1024, ">每个服务端物理连接对应的snet重连缓冲区大小。")
	serverSnetInitTimeout = flag.Duration("ServerSnetInitTimeout", 10*time.Second, ">服务端snet握手超时时间。")
	serverSnetWaitTimeout = flag.Duration("ServerSnetWaitTimeout", 60*time.Second, ">服务端snet等待重连超时时间。")
)

func main() {
	flag.Parse() //解析命令行参数写入注册的flag里。

	if *serverSendChanSize <= 0 { // "ServerSendChanSize", 102400, "每个服务端连接异步发送消息用的chan缓冲区大小
		println("server send chan size must greater than zero.")
	}

	var pool slab.Pool
	switch *memPoolType { // "memPoolType", "atom", "slab内存池类型 (sync、atom或chan)，默认atom"
	case "sync":
		pool = slab.NewSyncPool(*memPoolMinChunk, *memPoolMaxChunk, *memPoolFactor)
	case "atom":
		pool = slab.NewAtomPool(*memPoolMinChunk, *memPoolMaxChunk, *memPoolFactor, *memPoolPageSize)
	case "chan":
		pool = slab.NewChanPool(*memPoolMinChunk, *memPoolMaxChunk, *memPoolFactor, *memPoolPageSize)
	default:
		println(`unsupported memory pool type, must be "sync", "atom" or "chan"`)
	}

	gw := fastway.NewGateway(pool, *maxPacket) // 创建网关对象;"MaxPacket", 512*1024, "最大的消息包体积，默认512k

	go gw.ServeClients( // 开启客户端协程服务
		listen("client", *clientAddr, *reusePort, //:0 , false;是否开启reuseport特性，默认false
			*clientSnetEnable, // false ,是否为客户端开启snet协议
			*clientSnetEncrypt, // false, 是否为客户端开启snet加密
			*clientSnetBuffer, // 64*1024, 每个客户端物理连接对应的snet重连缓冲区大小
			*clientSnetInitTimeout, // 10*time.Second, 客户端snet握手超时时间
			*clientSnetWaitTimeout, // 60*time.Second, 客户端snet等待重连超时时间
		),
		fastway.GatewayCfg{
			MaxConn:      *clientMaxConn, // 16, 每个客户端可以创建的最大虚拟连接数
			BufferSize:   *clientBufferSize, // 2*1024, 每个客户端连接使用的 bufio.Reader 缓冲区大小
			SendChanSize: *clientSendChanSize, // 1024, 每个客户端连接异步发送消息用的chan缓冲区大小
			IdleTimeout:  *clientIdleTimeout, // 30*time.Second, 客户端连接空闲超过时长，客户端应在此时间范围内发送ping命令来保活
		},
	)

	go gw.ServeServers(
		listen("server", *serverAddr, *reusePort, // :0, 网关暴露给服务端的地址
			*serverSnetEnable, // false, 是否为服务端开启snet协议
			*serverSnetEncrypt, // false, 启用/禁用服务端开启snet加密
			*serverSnetBuffer, // 64*1024, 每个服务端物理连接对应的snet重连缓冲区大小
			*serverSnetInitTimeout, // 10*time.Second, 服务端snet握手超时时间
			*serverSnetWaitTimeout, // 60*time.Second, 服务端snet等待重连超时时间
		),
		fastway.GatewayCfg{
			AuthKey:      *serverAuthKey, // "", 用于验证服务端合法性的秘钥
			BufferSize:   *serverBufferSize, // 64*1024, 每个服务端连接使用的 bufio.Reader 缓冲区大小
			SendChanSize: *serverSendChanSize, // 102400, 每个服务端连接异步发送消息用的chan缓冲区大小
			IdleTimeout:  *serverIdleTimeout, // 30*time.Second, 服务端连接空闲超过时长，服务端应在此时间范围内发送ping命令来保活
		},
	)

	//cmd.Shell("fastway")

	gw.Stop()
}

func listen(who, addr string, reuse, snetEnable, snetEncrypt bool, snetBuffer int, snetInitTimeout, snetWaitTimeout time.Duration) net.Listener {
	var lsn net.Listener
	var err error

	if reuse {
		lsn, err = reuseport.NewReusablePortListener("tcp", addr)
	} else {
		lsn, err = net.Listen("tcp", addr)
	}

	if err != nil {
		log.Fatalf("setup %s listener at %s failed - %s", who, addr, err)
	}

	if snetEnable {
		lsn, _ = snet.Listen(snet.Config{
			EnableCrypt:        snetEncrypt,
			RewriterBufferSize: snetBuffer,
			HandshakeTimeout:   snetInitTimeout,
			ReconnWaitTimeout:  snetWaitTimeout,
		}, func() (net.Listener, error) {
			return lsn, nil
		})
	}

	log.Printf("setup %s listener at - %s", who, lsn.Addr())
	return lsn
}
