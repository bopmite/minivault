package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var hdrPool = sync.Pool{New: func() interface{} { return make([]byte, 5) }}

const (
	OpGet    = 0x01
	OpSet    = 0x02
	OpDelete = 0x03
	OpSync   = 0x04
	OpHealth = 0x05
	OpAuth   = 0x06
)

func writeErr(conn net.Conn) error {
	_, err := conn.Write([]byte{0xFF, 0, 0, 0, 0})
	return err
}

type BinaryServer struct {
	vault      *Vault
	authKey    string
	authMode   AuthMode
	rateLimit  int
	startTime  time.Time
	connSem    chan struct{}
	maxConn    int
	limiter    *rate.Limiter
}

func NewBinaryServer(vault *Vault, authKey string, authMode AuthMode, rateLimit int, startTime time.Time) *BinaryServer {
	maxConn := 50000
	sem := make(chan struct{}, maxConn)
	for i := 0; i < maxConn; i++ {
		sem <- struct{}{}
	}
	var limiter *rate.Limiter
	if rateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(rateLimit), rateLimit/10)
	}
	return &BinaryServer{
		vault:     vault,
		authKey:   authKey,
		authMode:  authMode,
		rateLimit: rateLimit,
		startTime: startTime,
		connSem:   sem,
		maxConn:   maxConn,
		limiter:   limiter,
	}
}

func (s *BinaryServer) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && !ne.Temporary() {
				return err
			}
			continue
		}

		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.SetNoDelay(true)
			tcp.SetReadBuffer(512 * 1024)
			tcp.SetWriteBuffer(512 * 1024)
		}

		select {
		case <-s.connSem:
			go func() {
				defer func() { s.connSem <- struct{}{} }()
				s.handle(conn)
			}()
		default:
			conn.Close()
		}
	}
}

func (s *BinaryServer) handle(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
		}
	}()
	defer conn.Close()

	authenticated := s.authMode == AuthNone
	hdr := make([]byte, 7)
	keyBuf := make([]byte, 0, 1024)
	valBuf := make([]byte, 0, 16384)

	for {
		if s.limiter != nil && !s.limiter.Allow() {
			if writeErr(conn) != nil {
				return
			}
			continue
		}

		if _, err := io.ReadFull(conn, hdr[:3]); err != nil {
			return
		}

		op := hdr[0]
		keyLen := binary.LittleEndian.Uint16(hdr[1:3])

		if cap(keyBuf) < int(keyLen) {
			keyBuf = make([]byte, keyLen)
		}
		keyBuf = keyBuf[:keyLen]
		if _, err := io.ReadFull(conn, keyBuf); err != nil {
			return
		}

		needsAuth := false
		if s.authMode == AuthAll && op != OpHealth && op != OpAuth {
			needsAuth = true
		} else if s.authMode == AuthWrites && (op == OpSet || op == OpDelete || op == OpSync) {
			needsAuth = true
		}

		if needsAuth && !authenticated {
			if op == OpSet || op == OpSync {
				if _, err := io.ReadFull(conn, hdr[:5]); err != nil {
					return
				}
				valLen := binary.LittleEndian.Uint32(hdr[:4])
				if valLen > uint32(MaxValueSize) {
					writeErr(conn)
					return
				}
				io.CopyN(io.Discard, conn, int64(valLen))
			}
			writeErr(conn)
			return
		}

		switch op {
		case OpAuth:
			if string(keyBuf) == s.authKey && s.authKey != "" {
				authenticated = true
				if _, err := conn.Write([]byte{0x00, 0, 0, 0, 0}); err != nil {
					return
				}
			} else {
				if writeErr(conn) != nil {
					return
				}
			}

		case OpGet:
			data, err := s.vault.storage.Get(string(keyBuf))
			if err != nil {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			respHdr := hdrPool.Get().([]byte)
			respHdr[0] = 0x00
			binary.LittleEndian.PutUint32(respHdr[1:], uint32(len(data)))
			if _, err := conn.Write(respHdr); err != nil {
				hdrPool.Put(respHdr)
				return
			}
			hdrPool.Put(respHdr)
			if _, err := conn.Write(data); err != nil {
				return
			}

		case OpSet:
			if _, err := io.ReadFull(conn, hdr[:5]); err != nil {
				return
			}
			valLen := binary.LittleEndian.Uint32(hdr[:4])
			compressed := hdr[4] == 1

			if valLen > uint32(MaxValueSize) {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			if cap(valBuf) < int(valLen) {
				valBuf = make([]byte, valLen)
			}
			valBuf = valBuf[:valLen]
			if _, err := io.ReadFull(conn, valBuf); err != nil {
				return
			}

			data, err := decompress(valBuf, compressed)
			if err != nil {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			if len(data) > MaxValueSize {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			if err := s.vault.cluster.write(string(keyBuf), data); err != nil {
				if writeErr(conn) != nil {
					return
				}
			} else {
				if _, err := conn.Write([]byte{0x00, 0, 0, 0, 0}); err != nil {
					return
				}
			}

		case OpDelete:
			if err := s.vault.cluster.delete(string(keyBuf)); err != nil {
				if writeErr(conn) != nil {
					return
				}
			} else {
				if _, err := conn.Write([]byte{0x00, 0, 0, 0, 0}); err != nil {
					return
				}
			}

		case OpSync:
			if _, err := io.ReadFull(conn, hdr[:5]); err != nil {
				return
			}
			valLen := binary.LittleEndian.Uint32(hdr[:4])
			compressed := hdr[4] == 1

			if valLen > uint32(MaxValueSize) {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			if cap(valBuf) < int(valLen) {
				valBuf = make([]byte, valLen)
			}
			valBuf = valBuf[:valLen]
			if _, err := io.ReadFull(conn, valBuf); err != nil {
				return
			}

			data, err := decompress(valBuf, compressed)
			if err != nil {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			if len(data) > MaxValueSize {
				if writeErr(conn) != nil {
					return
				}
				continue
			}

			if err := s.vault.storage.Set(string(keyBuf), data); err != nil {
				if writeErr(conn) != nil {
					return
				}
			} else {
				if _, err := conn.Write([]byte{0x00, 0, 0, 0, 0}); err != nil {
					return
				}
			}

		case OpHealth:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			health := map[string]interface{}{
				"status":          "healthy",
				"uptime_seconds":  int64(time.Since(s.startTime).Seconds()),
				"cache_items":     s.vault.storage.cache.items.Load(),
				"cache_size_mb":   s.vault.storage.cache.size.Load() / (1024 * 1024),
				"storage_size_mb": s.vault.storage.size.Load() / (1024 * 1024),
				"goroutines":      runtime.NumGoroutine(),
				"memory_mb":       m.Alloc / (1024 * 1024),
			}

			jsonData, _ := json.Marshal(health)

			respHdr := hdrPool.Get().([]byte)
			respHdr[0] = 0x00
			binary.LittleEndian.PutUint32(respHdr[1:], uint32(len(jsonData)))
			if _, err := conn.Write(respHdr); err != nil {
				hdrPool.Put(respHdr)
				return
			}
			hdrPool.Put(respHdr)
			if _, err := conn.Write(jsonData); err != nil {
				return
			}
		}
	}
}

type connPool struct {
	addr  string
	conns chan net.Conn
	mu    sync.Mutex
}

func newConnPool(addr string, size int) *connPool {
	return &connPool{
		addr:  addr,
		conns: make(chan net.Conn, size),
	}
}

func (p *connPool) Get() (net.Conn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dial()
	}
}

func (p *connPool) Put(conn net.Conn) {
	select {
	case p.conns <- conn:
	default:
		conn.Close()
	}
}

func (p *connPool) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", p.addr, 500*time.Millisecond)
	if err != nil {
		return nil, err
	}

	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetNoDelay(true)
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(30 * time.Second)
		tcp.SetReadBuffer(512 * 1024)
		tcp.SetWriteBuffer(512 * 1024)
	}

	return conn, nil
}

type BinaryClient struct {
	pools sync.Map
}

func NewBinaryClient() *BinaryClient {
	return &BinaryClient{}
}

func (c *BinaryClient) getPool(addr string) *connPool {
	if p, ok := c.pools.Load(addr); ok {
		return p.(*connPool)
	}

	pool := newConnPool(addr, 10)
	actual, _ := c.pools.LoadOrStore(addr, pool)
	return actual.(*connPool)
}

func (c *BinaryClient) Sync(addr, key, authKey string, data []byte) error {
	pool := c.getPool(addr)
	conn, err := pool.Get()
	if err != nil {
		return err
	}

	if authKey != "" {
		authReq := make([]byte, 3+len(authKey))
		authReq[0] = OpAuth
		binary.LittleEndian.PutUint16(authReq[1:3], uint16(len(authKey)))
		copy(authReq[3:], authKey)

		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write(authReq); err != nil {
			conn.Close()
			return err
		}
		authResp := make([]byte, 5)
		if _, err := io.ReadFull(conn, authResp); err != nil {
			conn.Close()
			return err
		}
		if authResp[0] != 0x00 {
			conn.Close()
			return fmt.Errorf("auth failed")
		}
	}

	compressed := compress(data)
	isCompressed := len(compressed) < len(data)
	if !isCompressed {
		compressed = data
	}

	req := make([]byte, 3+len(key)+5+len(compressed))
	req[0] = OpSync
	binary.LittleEndian.PutUint16(req[1:3], uint16(len(key)))
	copy(req[3:], key)
	binary.LittleEndian.PutUint32(req[3+len(key):], uint32(len(compressed)))
	if isCompressed {
		req[3+len(key)+4] = 1
	}
	copy(req[3+len(key)+5:], compressed)

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return err
	}

	resp := make([]byte, 5)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return err
	}
	conn.SetDeadline(time.Time{})

	pool.Put(conn)
	if resp[0] != 0x00 {
		return fmt.Errorf("sync failed")
	}
	return nil
}

func (c *BinaryClient) Get(addr, key string) ([]byte, error) {
	pool := c.getPool(addr)
	conn, err := pool.Get()
	if err != nil {
		return nil, err
	}

	req := make([]byte, 3+len(key))
	req[0] = OpGet
	binary.LittleEndian.PutUint16(req[1:3], uint16(len(key)))
	copy(req[3:], key)

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	resp := make([]byte, 5)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, err
	}

	if resp[0] != 0x00 {
		conn.SetDeadline(time.Time{})
		pool.Put(conn)
		return nil, fmt.Errorf("not found")
	}

	dataLen := binary.LittleEndian.Uint32(resp[1:])
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(conn, data); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})

	pool.Put(conn)
	return data, nil
}

func (c *BinaryClient) Delete(addr, key, authKey string) error {
	pool := c.getPool(addr)
	conn, err := pool.Get()
	if err != nil {
		return err
	}

	if authKey != "" {
		authReq := make([]byte, 3+len(authKey))
		authReq[0] = OpAuth
		binary.LittleEndian.PutUint16(authReq[1:3], uint16(len(authKey)))
		copy(authReq[3:], authKey)

		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write(authReq); err != nil {
			conn.Close()
			return err
		}
		authResp := make([]byte, 5)
		if _, err := io.ReadFull(conn, authResp); err != nil {
			conn.Close()
			return err
		}
		if authResp[0] != 0x00 {
			conn.Close()
			return fmt.Errorf("auth failed")
		}
	}

	req := make([]byte, 3+len(key))
	req[0] = OpDelete
	binary.LittleEndian.PutUint16(req[1:3], uint16(len(key)))
	copy(req[3:], key)

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return err
	}

	resp := make([]byte, 5)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return err
	}
	conn.SetDeadline(time.Time{})

	pool.Put(conn)
	return nil
}
