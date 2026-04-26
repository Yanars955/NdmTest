package main

import (
	"container/list"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type Queue struct {
	mu   sync.Mutex
	list *list.List
	wait *list.List
}

var queues = make(map[string]*Queue)
var qmu sync.Mutex

func getQueue(name string) *Queue {
	qmu.Lock()
	q, ok := queues[name]
	if !ok {
		q = &Queue{list: list.New(), wait: list.New()}
		queues[name] = q
	}
	qmu.Unlock()
	return q
}

func putHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if v := r.URL.Query().Get("v"); v != "" {
		q := getQueue(r.URL.Path[1:])
		q.mu.Lock()
		if q.wait.Len() > 0 {
			ch := q.wait.Remove(q.wait.Front()).(chan string)
			ch <- v
		} else {
			q.list.PushBack(v)
		}
		q.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[1:]
	q := getQueue(name)

	timeout := time.Duration(0)
	if t := r.URL.Query().Get("timeout"); t != "" {
		var err error
		timeout, err = time.ParseDuration(t + "s")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	q.mu.Lock()

	if q.list.Len() > 0 {
		msg := q.list.Remove(q.list.Front()).(string)
		w.Write([]byte(msg))
		q.mu.Unlock()
		return
	}

	if timeout == 0 {
		q.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ch := make(chan string, 1)
	q.wait.PushBack(ch)
	q.mu.Unlock()

	select {
	case msg := <-ch:
		w.Write([]byte(msg))
	case <-time.After(timeout):
		q.mu.Lock()
		if q.wait.Len() > 0 && q.wait.Back() != nil {
			q.wait.Remove(q.wait.Back())
		}
		q.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <port>", os.Args[0])
		os.Exit(1)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			putHandler(w, r)
		} else {
			getHandler(w, r)
		}
	})
	http.ListenAndServe(":"+os.Args[1], nil)
}
