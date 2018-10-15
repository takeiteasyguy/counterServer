package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type counter struct {
	conns    []time.Time
	m        sync.RWMutex
	rcvchan  chan bool
	errchan  chan<- error
	fileName string
}

// newCounter is a counter constructor
func newCounter(rcvchan chan bool, errchan chan<- error, fileName string) (*counter, error) {
	ctr := &counter{
		conns:    make([]time.Time, 0),
		m:        sync.RWMutex{},
		rcvchan:  rcvchan,
		errchan:  errchan,
		fileName: fileName,
	}
	err := ctr.Load()
	if err != nil {
		return ctr, err
	}
	return ctr, nil
}

// Run is a method that process removing of out-of-date connections and adding a new ones
func (c *counter) Run() {
	go c.check()
	go c.append()
}

func (c *counter) counter() int {
	c.m.RLock()
	cc := len(c.conns)
	c.m.RUnlock()
	return cc
}

func (c *counter) check() {
	for {
		i := 0
		c.m.Lock()
		j := len(c.conns)
		for i < j {
			if time.Since(c.conns[i]) > timeRequestsStored {
				c.conns[i] = c.conns[len(c.conns)-1]
				c.conns[len(c.conns)-1] = time.Time{}
				c.conns = c.conns[:len(c.conns)-1]
				j--
			} else {
				i++
			}
		}
		c.m.Unlock()
	}
}

func (c *counter) append() {
	f, err := os.OpenFile(c.fileName, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		c.errchan <- err
	}
	defer f.Close()
	for {
		select {
		case <-c.rcvchan:
			t := time.Now()
			c.m.Lock()
			c.conns = append(c.conns, t)
			c.m.Unlock()
			// it's better to store request time as a binary data as it saves more space on hard drive
			b, err := t.MarshalBinary()
			if err != nil {
				c.errchan <- err
			}
			// Truncating file and storing only valid requests is not possible here as it's not guaranteed
			// at what time main process could be killed
			// Depending on this data necessity it could be backed up to other file temporary or other solution could be provided
			_, err = f.Write(b)
			f.WriteString("\n")
			if err != nil {
				c.errchan <- err
			}
		}
	}
}

// Load method gets the data from persistent storage file and applies it to in-memory solution; also re-store only valid requests
func (c *counter) Load() error {
	f, err := os.OpenFile(c.fileName, os.O_RDWR|os.O_CREATE, 0600)
	defer f.Close()
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		t := time.Time{}
		err := t.UnmarshalBinary(sc.Bytes())
		if err != nil {
			// Error here could occur in case if binary data in persistent data file is corrupted
			// That does not mean that all of the requests connection time is corrupted
			// This could occur cause the server stopped incorrectly and appending new binary data was not fully finished
			log.Printf("Persistent Data file has corrupted binary, cleared forcelly: %s", err.Error())
		}
		if time.Since(t) < timeRequestsStored {
			c.conns = append(c.conns, t)
		}
	}
	if err = sc.Err(); err != nil {
		return err
	}
	// Opposite to append method while Load it's possible to remove non-valid requests
	err = f.Truncate(0)
	if err != nil {
		return err
	}
	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}
	for _, ct := range c.conns {
		b, err := ct.MarshalBinary()
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		f.WriteString("\n")
		if err != nil {
			return err
		}
	}
	return err
}

// Handle is an HTTP handler method
func (c *counter) Handle(w http.ResponseWriter, r *http.Request) {
	rr := c.counter() + 1
	c.rcvchan <- true
	b, err := json.Marshal(map[string]int{"request_counter": rr})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(b)
	log.Printf("Request from %s has been handled", r.RemoteAddr)
}
