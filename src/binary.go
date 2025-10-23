package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	OpGet    = 0x01
	OpSet    = 0x02
	OpDelete = 0x03
	OpSync   = 0x04
)

type BinaryServer struct {
	vault *Vault
}

func NewBinaryServer(vault *Vault) *BinaryServer {
	return &BinaryServer{vault: vault}
}

func (s *BinaryServer) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.SetNoDelay(true)
			tcp.SetReadBuffer(512 * 1024)
			tcp.SetWriteBuffer(512 * 1024)
		}

		go s.handle(conn)
	}
}

func (s *BinaryServer) handle(conn net.Conn) {
	defer conn.Close()

	hdr := make([]byte, 7)
	keyBuf := make([]byte, 0, 1024)
	valBuf := make([]byte, 0, 16384)

	for {
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

		switch op {
		case OpGet:
			data, err := s.vault.storage.Get(string(keyBuf))
			if err != nil {
				conn.Write([]byte{0xFF, 0, 0, 0, 0})
				continue
			}

			respHdr := make([]byte, 5)
			respHdr[0] = 0x00
			binary.LittleEndian.PutUint32(respHdr[1:], uint32(len(data)))
			conn.Write(respHdr)
			conn.Write(data)

		case OpSet:
			if _, err := io.ReadFull(conn, hdr[:5]); err != nil {
				return
			}
			valLen := binary.LittleEndian.Uint32(hdr[:4])
			compressed := hdr[4] == 1

			if cap(valBuf) < int(valLen) {
				valBuf = make([]byte, valLen)
			}
			valBuf = valBuf[:valLen]
			if _, err := io.ReadFull(conn, valBuf); err != nil {
				return
			}

			data, err := decompress(valBuf, compressed)
			if err != nil {
				conn.Write([]byte{0xFF, 0, 0, 0, 0})
				continue
			}

			if err := s.vault.cluster.write(string(keyBuf), data); err != nil {
				conn.Write([]byte{0xFF, 0, 0, 0, 0})
			} else {
				conn.Write([]byte{0x00, 0, 0, 0, 0})
			}

		case OpDelete:
			s.vault.storage.Delete(string(keyBuf))
			nodes := s.vault.cluster.hash(string(keyBuf), ReplicaCount)
			for _, node := range nodes {
				if node != s.vault.cluster.self {
					go s.vault.cluster.sendDelete(node, string(keyBuf))
				}
			}
			conn.Write([]byte{0x00, 0, 0, 0, 0})

		case OpSync:
			if _, err := io.ReadFull(conn, hdr[:5]); err != nil {
				return
			}
			valLen := binary.LittleEndian.Uint32(hdr[:4])
			compressed := hdr[4] == 1

			if cap(valBuf) < int(valLen) {
				valBuf = make([]byte, valLen)
			}
			valBuf = valBuf[:valLen]
			if _, err := io.ReadFull(conn, valBuf); err != nil {
				return
			}

			data, err := decompress(valBuf, compressed)
			if err != nil {
				conn.Write([]byte{0xFF, 0, 0, 0, 0})
				continue
			}

			if err := s.vault.storage.Set(string(keyBuf), data); err != nil {
				conn.Write([]byte{0xFF, 0, 0, 0, 0})
			} else {
				conn.Write([]byte{0x00, 0, 0, 0, 0})
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

func (c *BinaryClient) Sync(addr, key string, data []byte) error {
	pool := c.getPool(addr)
	conn, err := pool.Get()
	if err != nil {
		return err
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

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return err
	}

	resp := make([]byte, 5)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return err
	}

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
		pool.Put(conn)
		return nil, fmt.Errorf("not found")
	}

	dataLen := binary.LittleEndian.Uint32(resp[1:])
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(conn, data); err != nil {
		conn.Close()
		return nil, err
	}

	pool.Put(conn)
	return data, nil
}

func (c *BinaryClient) Delete(addr, key string) error {
	pool := c.getPool(addr)
	conn, err := pool.Get()
	if err != nil {
		return err
	}

	req := make([]byte, 3+len(key))
	req[0] = OpDelete
	binary.LittleEndian.PutUint16(req[1:3], uint16(len(key)))
	copy(req[3:], key)

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return err
	}

	resp := make([]byte, 5)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return err
	}

	pool.Put(conn)
	return nil
}

type RateLimiter struct {
	tokens   atomic.Uint64
	rate     uint64
	capacity uint64
	last     atomic.Int64
}

func NewRateLimiter(rate int) *RateLimiter {
	rl := &RateLimiter{
		rate:     uint64(rate),
		capacity: uint64(rate),
	}
	rl.tokens.Store(uint64(rate))
	rl.last.Store(time.Now().UnixNano())
	return rl
}

func (rl *RateLimiter) Allow() bool {
	now := time.Now().UnixNano()
	last := rl.last.Swap(now)
	elapsed := float64(now-last) / 1e9

	refill := uint64(elapsed * float64(rl.rate))

	for {
		tokens := rl.tokens.Load()
		newTokens := tokens + refill
		if newTokens > rl.capacity {
			newTokens = rl.capacity
		}

		if newTokens < 1 {
			rl.tokens.Store(newTokens)
			return false
		}

		if rl.tokens.CompareAndSwap(tokens, newTokens-1) {
			return true
		}
	}
}
