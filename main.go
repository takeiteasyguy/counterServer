package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fs := flag.NewFlagSet("srv", flag.ExitOnError)

	host := fs.String("host", "127.0.0.1", "http host")
	port := fs.String("port", "8080", "http listen port")
	file := fs.String("file", "persistent.data", "file which will be used to store request count")

	rcvchan := make(chan bool)
	errchan := make(chan error)

	go func() {
		for {
			select {
			case err := <-errchan:
				log.Fatalf(err.Error())
			}
		}
	}()

	ctr, err := newCounter(rcvchan, errchan, *file)
	if err != nil {
		log.Fatalf("Error while loading data from file: %s", err.Error())
	}

	srv := &http.Server{
		Handler: http.HandlerFunc(ctr.Handle),
		Addr:    net.JoinHostPort(*host, *port),
	}

	go ctr.Run()

	var stopSignal = make(chan os.Signal)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()
	log.Printf("Server is listening at %s:%s/", *host, *port)

	log.Println("received system interruption:", "signal", <-stopSignal)
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("the server cannot be stopped correctly: %v", err)
	}
}
