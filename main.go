package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	if err := command(); err != nil {
		slog.Error("failed to run command", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

var (
	addr  string
	hosts = []string{"localhost"}
	ports = []uint16{22, 80, 443}

	maxConn     = 100
	connTimeout = 10 * time.Second

	cacheExpires         = 72 * time.Hour
	openPortCacheExpires = 15 * time.Minute

	dbPath = ".cache"
	db     *badger.DB

	openPorts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "open_port",
			Help: "Status of open ports (1 - open)",
		},
		[]string{"host", "port"},
	)
)

func command() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	flag.StringVar(&addr, "web.listen-address", ":9116", "listen address")
	flag.Func("hosts", "hosts to scan ports for (comma-separated)", parseHosts())
	flag.Func("ports", "ports to scan (80,443, 100-200)", parsePorts())
	flag.Func("list", "list of hosts to scan ports (file)", parseHostsListFile())
	flag.IntVar(&maxConn, "max-connections", maxConn, "maximum number of connections")
	flag.DurationVar(&connTimeout, "timeout", connTimeout, "timeout for connection")
	flag.DurationVar(&cacheExpires, "cache-expires", cacheExpires, "cache expiration time")
	flag.DurationVar(&openPortCacheExpires, "open-port-cache-expires", openPortCacheExpires, "open port cache expiration time")
	flag.StringVar(&dbPath, "cache-path", dbPath, "path to cache database")
	flag.Parse()

	slog.Info("listening", slog.String("address", addr))
	slog.Info("number of hosts", slog.Int("count", len(hosts)))
	slog.Info("number of ports", slog.Int("count", len(ports)))
	slog.Info("max connections", slog.Int("count", maxConn))
	slog.Info("timeout", slog.Duration("timeout", connTimeout))

	prometheus.MustRegister(openPorts)

	var err error

	dbOpts := badger.DefaultOptions(dbPath)
	dbOpts.Logger = nil

	db, err = badger.Open(dbOpts)
	if err != nil {
		return err
	}
	defer db.Close()

	go scanner(ctx)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		slog.Info("shutting down server")

		_ = srv.Shutdown(ctx)
	}()

	if err = srv.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
	return nil
}

func scanner(ctx context.Context) {
	scanAll(ctx)

	var interval time.Duration
	if openPortCacheExpires < cacheExpires {
		interval = openPortCacheExpires
	} else {
		interval = cacheExpires
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			scanAll(ctx)
		}
	}
}

func scanAll(ctx context.Context) {
	semaphore := make(chan struct{}, maxConn)
	defer close(semaphore)

	wg := sync.WaitGroup{}

top:
	for _, host := range hosts {
		for _, port := range ports {
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				break top
			}

			wg.Add(1)

			go func(host string, port uint16) {
				defer wg.Done()
				defer func() { <-semaphore }()

				if open, err := scanWithCache(fmt.Sprintf("%s:%d", host, port)); err != nil {
					slog.Error("failed to scan", slog.String("host", host), slog.Int("port", int(port)), slog.String("error", err.Error()))
					return
				} else if open {
					openPorts.WithLabelValues(host, strconv.Itoa(int(port))).Set(1)
					slog.Info("open port", slog.String("host", host), slog.Int("port", int(port)))
				} else {
					slog.Debug("closed port", slog.String("host", host), slog.Int("port", int(port)))
				}
			}(host, port)
		}
	}

	wg.Wait()
}

func parseHostsListFile() func(s string) error {
	return func(s string) error {
		f, err := os.Open(s)
		if err != nil {
			return err
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			hosts = append(hosts, sc.Text())
		}
		return sc.Err()
	}
}

func parsePorts() func(s string) error {
	return func(s string) error {
		for _, p := range strings.Split(s, ",") {
			if strings.Contains(p, "-") {
				var start, end uint16
				_, err := fmt.Sscanf(p, "%d-%d", &start, &end)
				if err != nil {
					return err
				}
				for i := start; i <= end; i++ {
					ports = append(ports, i)
				}
			} else {
				port, err := strconv.Atoi(p)
				if err != nil {
					return err
				}
				ports = append(ports, uint16(port))
			}
		}
		return nil
	}
}

func parseHosts() func(s string) error {
	return func(s string) error {
		hosts = strings.Split(s, ",")
		return nil
	}
}

func getCache(addr string) (open bool, ok bool, err error) {
	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(addr))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			return err
		}
		return item.Value(func(val []byte) error {
			ok = true
			open = val[0] == 1
			return nil
		})
	})
	return
}

func setCache(addr string, open bool) error {
	entry := badger.NewEntry([]byte(addr), make([]byte, 1))

	if open {
		entry.Value[0] = 1
		entry.WithTTL(openPortCacheExpires)
	} else {
		entry.WithTTL(cacheExpires)
	}

	return db.Update(func(txn *badger.Txn) error {
		return txn.SetEntry(entry)
	})
}

func scanWithCache(addr string) (bool, error) {
	open, ok, err := getCache(addr)
	if err != nil {
		return false, err
	}

	if ok {
		return open, nil
	}

	if open, err = scan(addr); err != nil {
		return false, err
	}

	if err = setCache(addr, open); err != nil {
		return false, err
	}

	return open, nil
}

func scan(addr string) (bool, error) {
	c, err := net.DialTimeout("tcp", addr, connTimeout)
	if err != nil {
		if strings.Contains(err.Error(), "too many open files") {
			return false, err
		}
	}
	c.Close()
	return true, nil
}
