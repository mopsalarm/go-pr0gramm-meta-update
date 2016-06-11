package main

import (
	"database/sql"
	"flag"
	"time"

	"fmt"
	"github.com/Sirupsen/logrus"
	_ "github.com/lib/pq"
	"github.com/rcrowley/go-metrics"
	"github.com/robfig/cron"
	"github.com/vistarmedia/go-datadog"
	"os"
)

func main() {
	postgres := flag.String("postgres", "host=localhost user=postgres password=password dbname=postgres sslmode=disable", "Postgres connection string or url")
	datadog := flag.String("datadog", "", "Datadog api key for metrics reporter")
	flag.Parse()

	db, err := sql.Open("postgres", *postgres)
	if err != nil {
		logrus.WithError(err).Fatal("Could not connect to database")
		return
	}

	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		logrus.WithError(err).Fatal("Could not ping database.")
		return
	}

	if *datadog != "" {
		startMetricsWithDatadog(*datadog)
	}

	scheduleUpdateFunctions(db)

	// wait forever.
	<-make(chan bool)
}

func scheduleUpdateFunctions(db *sql.DB) {
	c := cron.New()
	must(c.AddFunc("@every 10m", func() {
		Update(db, 6*time.Hour)
	}))

	must(c.AddFunc("@hourly", func() {
		Update(db, 24*time.Hour)
	}))

	must(c.AddFunc("@every 3h", func() {
		Update(db, 7*24*time.Hour)
	}))

	// update "today" once
	Update(db, 24*time.Hour)

	// start update cycle
	c.Start()
}

func startMetricsWithDatadog(apiKey string) {
	metrics.RegisterRuntimeMemStats(metrics.DefaultRegistry)
	go metrics.CaptureRuntimeMemStats(metrics.DefaultRegistry, 10)

	host, _ := os.Hostname()

	fmt.Printf("Starting datadog reporter on host %s\n", host)
	go datadog.New(host, apiKey).DefaultReporter().Start(10)
}

func must(err error) {
	if err != nil {
		logrus.WithError(err).Fatal("An error occured")
	}
}
