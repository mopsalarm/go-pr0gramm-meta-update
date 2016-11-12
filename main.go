package main

import (
	"database/sql"
	"flag"
	"time"

	"fmt"
	"github.com/Sirupsen/logrus"
	_ "github.com/lib/pq"
	"github.com/mopsalarm/go-pr0gramm"
	"github.com/rcrowley/go-metrics"
	"github.com/robfig/cron"
	"github.com/vistarmedia/go-datadog"
	"net/http"
	"os"
	"sync/atomic"
)

func main() {
	postgres := flag.String("postgres", "host=localhost user=postgres password=password dbname=postgres sslmode=disable", "Postgres connection string or url")
	datadog := flag.String("datadog", "", "Datadog api key for metrics reporter")
	updateAll := flag.Bool("all", false, "Update everything (slowly)")

	flag.Parse()

	db, err := sql.Open("postgres", *postgres)
	if err != nil {
		logrus.WithError(err).Fatal("Could not connect to database")
		return
	}

	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		logrus.WithError(err).Fatal("Could not ping database.")
		return
	}

	if *datadog != "" {
		startMetricsWithDatadog(*datadog)
	}

	// if we have such a slow connection, servers are fucked up.
	http.DefaultClient.Timeout = 10 * time.Second

	if *updateAll {
		// update everything
		request := pr0gramm.NewItemsRequest()

		for {
			id, err := UpdateAll(db, request)
			if err == nil {
				break
			}

			logrus.WithError(err).WithField("id", id).Warn("Error updating items")
			request.Older = id - pr0gramm.Id(1)

			time.Sleep(20 * time.Second)
		}

	} else {
		scheduleUpdateFunctions(db)
	}

	// wait forever.
	<-make(chan bool)
}

func limitConcurrency(fn func()) func() {
	var guard int32
	return func() {
		if atomic.CompareAndSwapInt32(&guard, 0, 1) {
			defer atomic.StoreInt32(&guard, 0)
			fn()
		}
	}
}

func scheduleUpdateFunctions(db *sql.DB) {
	c := cron.New()
	must(c.AddFunc("@every 1m", limitConcurrency(func() {
		Update(db, time.Hour)
	})))

	must(c.AddFunc("@every 10m", limitConcurrency(func() {
		Update(db, 6*time.Hour)
	})))

	must(c.AddFunc("@hourly", limitConcurrency(func() {
		Update(db, 24*time.Hour)
	})))

	must(c.AddFunc("@every 3h", limitConcurrency(func() {
		Update(db, 7*24*time.Hour)
	})))

	must(c.AddFunc("@every 15s", limitConcurrency(func() {
		UpdateTags(db)
	})))

	// update "today" once
	Update(db, 24*time.Hour)

	// start update cycle
	c.Start()
}

func startMetricsWithDatadog(apiKey string) {
	metrics.RegisterRuntimeMemStats(metrics.DefaultRegistry)
	go metrics.CaptureRuntimeMemStats(metrics.DefaultRegistry, 1*time.Minute)

	host, _ := os.Hostname()

	fmt.Printf("Starting datadog reporter on host %s\n", host)
	go datadog.New(host, apiKey).DefaultReporter().Start(1 * time.Minute)
}

func must(err error) {
	if err != nil {
		logrus.WithError(err).Fatal("An error occured")
	}
}
