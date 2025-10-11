package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	walMagic    = 0xDEAD
	walMaxBatch = 1000
	walFlushMs  = 1
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
	w.ch <- walEntry{hash: h, data: data}
}

func (w *wal) flusher() {
	ticker := time.NewTicker(walFlushMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case e := <-w.ch:
			w.batch = append(w.batch, e)
			if len(w.batch) >= walMaxBatch {
				w.flush()
			}
		case <-ticker.C:
			if len(w.batch) > 0 {
				w.flush()
			}
		case <-w.done:
			w.flush()
			w.file.Close()
			return
		}
	}
}

func (w *wal) flush() {
	if len(w.batch) == 0 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var buf [16]byte
	for _, e := range w.batch {
		binary.LittleEndian.PutUint16(buf[0:2], walMagic)
		binary.LittleEndian.PutUint64(buf[2:10], e.hash)
		binary.LittleEndian.PutUint32(buf[10:14], uint32(len(e.data)))
		binary.LittleEndian.PutUint16(buf[14:16], uint16(hash64(e.data)&0xFFFF))

		w.file.Write(buf[:])
		w.file.Write(e.data)
	}

	w.file.Sync()
	w.batch = w.batch[:0]
}

func (w *wal) close() {
	close(w.done)
}

func (w *wal) compact() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	old := w.file
	path := filepath.Join(w.dir, "wal.log")
	temp := path + ".tmp"

	f, err := os.Create(temp)
	if err != nil {
		return err
	}

	w.file = f
	old.Close()
	os.Remove(path)
	return os.Rename(temp, path)
}
