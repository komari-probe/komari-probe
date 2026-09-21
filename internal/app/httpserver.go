package app

import (
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// serveInBackground starts server.ListenAndServe on its own goroutine and
// reports any error other than a clean shutdown on the returned channel.
func serveInBackground(server *http.Server) <-chan error {
	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	return serverErr
}

// watchQuitSignal reports SIGINT/SIGTERM on the returned channel. The
// returned stop function must be called (typically via defer) to release the
// underlying signal registration.
func watchQuitSignal() (<-chan os.Signal, func()) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	return quit, func() { signal.Stop(quit) }
}
