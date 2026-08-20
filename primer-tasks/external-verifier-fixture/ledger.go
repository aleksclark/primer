package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type ledgerFile struct {
	Requests  map[string]requestRecord  `json:"requests"`
	Callbacks map[string]callbackRecord `json:"callbacks"`
}

type ledger struct {
	mu   sync.Mutex
	path string
	data ledgerFile
}

func openLedger(path string) (*ledger, error) {
	l := &ledger{path: path, data: ledgerFile{Requests: map[string]requestRecord{}, Callbacks: map[string]callbackRecord{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &l.data); err != nil {
		return nil, err
	}
	if l.data.Requests == nil {
		l.data.Requests = map[string]requestRecord{}
	}
	if l.data.Callbacks == nil {
		l.data.Callbacks = map[string]callbackRecord{}
	}
	return l, nil
}

func (l *ledger) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(l.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".ledger-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, l.path)
}

func (l *ledger) putRequest(r requestRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data.Requests[r.Request.IdempotencyKey] = r
	return l.saveLocked()
}

func (l *ledger) findRequest(key string) (requestRecord, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.data.Requests[key]
	return r, ok
}

func (l *ledger) updateRequest(key string, update func(*requestRecord)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.data.Requests[key]
	if !ok {
		return errors.New("request not found")
	}
	update(&r)
	l.data.Requests[key] = r
	return l.saveLocked()
}

func (l *ledger) putCallback(c callbackRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if old, ok := l.data.Callbacks[c.CallbackID]; ok {
		c.Attempts = old.Attempts
	}
	l.data.Callbacks[c.CallbackID] = c
	return l.saveLocked()
}

func (l *ledger) updateCallback(id string, update func(*callbackRecord)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.data.Callbacks[id]
	if !ok {
		return errors.New("callback not found")
	}
	update(&c)
	l.data.Callbacks[id] = c
	return l.saveLocked()
}

func (l *ledger) snapshot() ledgerFile {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := ledgerFile{Requests: map[string]requestRecord{}, Callbacks: map[string]callbackRecord{}}
	for k, v := range l.data.Requests {
		v.Request.Payload = nil
		out.Requests[k] = v
	}
	for k, v := range l.data.Callbacks {
		out.Callbacks[k] = v
	}
	return out
}
