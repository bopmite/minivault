package main

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	walMagic      = 0xDEAD
	walMaxBatch   = 1000
	walFlushMs    = 10
	walMaxBytes   = 1024 * 1024
	walCompactMin = 40 * 1024
)

type walEntry struct {
	hash uint64
	data []byte
}

type wal struct {
	dir   string
	file  *os.File
	mu    sync.Mutex
	batch []walEntry
	ch    chan walEntry
	done  chan struct{}
}

func newWAL(dir string) (*wal, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "wal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	w := &wal{
		dir:   dir,
		file:  f,
		batch: make([]walEntry, 0, walMaxBatch),
		ch:    make(chan walEntry, walMaxBatch*2),
		done:  make(chan struct{}),
	}

	go w.flusher()
	return w, nil
}

func (w *wal) append(h uint64, data []byte) {
	select {
	case w.ch <- walEntry{hash: h, data: data}:
	default:
	}
}

func (w *wal) flusher() {
	ticker := time.NewTicker(walFlushMs * time.Millisecond)
	defer ticker.Stop()
	bytes := 0

	for {
		select {
		case e := <-w.ch:
			w.mu.Lock()
			w.batch = append(w.batch, e)
			bytes += len(e.data)
			if len(w.batch) >= walMaxBatch || bytes >= walMaxBytes {
				w.flushLocked()
				bytes = 0
			}
			w.mu.Unlock()
		case <-ticker.C:
			w.mu.Lock()
			if len(w.batch) > 0 {
				w.flushLocked()
				bytes = 0
			}
			w.mu.Unlock()
		case <-w.done:
			w.mu.Lock()
			w.flushLocked()
			w.mu.Unlock()
			w.file.Close()
			return
		}
	}
}

func (w *wal) flushLocked() {
	if len(w.batch) == 0 {
		return
	}

	dedup := make(map[uint64][]byte, len(w.batch))
	for _, e := range w.batch {
		dedup[e.hash] = e.data
	}

	var buf [16]byte
	for hash, data := range dedup {
		binary.LittleEndian.PutUint16(buf[0:2], walMagic)
		binary.LittleEndian.PutUint64(buf[2:10], hash)
		binary.LittleEndian.PutUint32(buf[10:14], uint32(len(data)))
		binary.LittleEndian.PutUint16(buf[14:16], uint16(crc32.ChecksumIEEE(data)&0xFFFF))

		w.file.Write(buf[:])
		w.file.Write(data)
	}

	w.file.Sync()
	w.batch = w.batch[:0]

	if info, err := w.file.Stat(); err == nil && info.Size() > walCompactMin {
		w.compact()
	}
}

func (w *wal) compact() {
	entries := make(map[uint64][]byte)
	w.replay(func(h uint64, data []byte) error {
		if len(data) == 0 {
			delete(entries, h)
		} else {
			entries[h] = data
		}
		return nil
	})

	tmpPath := filepath.Join(w.dir, "wal.log.tmp")
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}

	var buf [16]byte
	for hash, data := range entries {
		if len(data) > 0 {
			binary.LittleEndian.PutUint16(buf[0:2], walMagic)
			binary.LittleEndian.PutUint64(buf[2:10], hash)
			binary.LittleEndian.PutUint32(buf[10:14], uint32(len(data)))
			binary.LittleEndian.PutUint16(buf[14:16], uint16(crc32.ChecksumIEEE(data)&0xFFFF))

			if _, err := f.Write(buf[:]); err != nil {
				f.Close()
				os.Remove(tmpPath)
				return
			}
			if _, err := f.Write(data); err != nil {
				f.Close()
				os.Remove(tmpPath)
				return
			}
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return
	}
	f.Close()

	w.file.Close()
	path := filepath.Join(w.dir, "wal.log")
	if err := os.Rename(tmpPath, path); err != nil {
		return
	}

	newFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	w.file = newFile
}

func (w *wal) close() {
	close(w.done)
}

func (w *wal) replay(fn func(h uint64, data []byte) error) error {
	path := filepath.Join(w.dir, "wal.log")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	var buf [16]byte
	for {
		if _, err := io.ReadFull(f, buf[:]); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		magic := binary.LittleEndian.Uint16(buf[0:2])
		if magic != walMagic {
			break
		}

		hash := binary.LittleEndian.Uint64(buf[2:10])
		dataLen := binary.LittleEndian.Uint32(buf[10:14])
		checksum := binary.LittleEndian.Uint16(buf[14:16])

		data := make([]byte, dataLen)
		if _, err := io.ReadFull(f, data); err != nil {
			return err
		}

		if uint16(crc32.ChecksumIEEE(data)&0xFFFF) != checksum {
			break
		}

		if err := fn(hash, data); err != nil {
			return err
		}
	}

	return nil
}

func (w *wal) truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.file.Close()

	path := filepath.Join(w.dir, "wal.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	w.file = f
	return nil
}
