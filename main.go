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
	updateStart := flag.Int("start-at", 0, "Specifiy the post number to start at when doing the complete import.")

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

		if *updateStart > 0 {
			request.Older = pr0gramm.Id(*updateStart)
		}

		for {
			logrus.Infof("Starting at id=%d", request.Older)
			id, err := UpdateAll(db, request)
			if err == nil {
				break
			}

			logrus.WithError(err).WithField("id", id).Warn("Error updating items")
			request.Older = id - 1

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

func printUpdateTime(title string, startTime time.Time) {
	logrus.Infof("Update '%s' took %s", title, time.Since(startTime))
}

func scheduleUpdateFunctions(db *sql.DB) {
	c := cron.New()
	must(c.AddFunc("30 * * * *", limitConcurrency(func() {
		defer printUpdateTime("6h every minute", time.Now())
		Update(db, 6*time.Hour)
	})))

	must(c.AddFunc("0 */15 * * *", limitConcurrency(func() {
		defer printUpdateTime("2d every 15min", time.Now())
		Update(db, 48*time.Hour)
	})))

	// once every hour (on +25m)
	must(c.AddFunc("0 25 * * *", limitConcurrency(func() {
		defer printUpdateTime("3d every hour", time.Now())
		Update(db, 7*24*time.Hour)
	})))

	// once a day (on +50m) we go back for a month
	must(c.AddFunc("0 50 4 * *", limitConcurrency(func() {
		defer printUpdateTime("30d every day", time.Now())
		Update(db, 30*24*time.Hour)
	})))

	must(c.AddFunc("@every 15s", limitConcurrency(func() {
		UpdateTags(db)
	})))

	// update the past two days once
	defer printUpdateTime("2d now", time.Now())
	Update(db, 48*time.Hour)

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
