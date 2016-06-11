package main

import (
	"database/sql"
	"flag"
	"time"

	"github.com/Sirupsen/logrus"
	_ "github.com/lib/pq"
	"github.com/robfig/cron"
)

func main() {
	postgres := flag.String("postgres", "host=localhost user=postgres password=password dbname=postgres sslmode=disable", "Postgres connection string or url")
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

	c := cron.New()
	must(c.AddFunc("@every 10m", func() {
		Update(db, 6 * time.Hour)
	}))

	must(c.AddFunc("@hourly", func() {
		Update(db, 24 * time.Hour)
	}))

	must(c.AddFunc("@every 3h", func() {
		Update(db, 7 * 24 * time.Hour)
	}))

	// update "today"
	Update(db, 24 * time.Hour)

	// start update cycle
	c.Start()

	// wait forever.
	<-make(chan bool)
}

func must(err error) {
	if err != nil {
		logrus.WithError(err).Fatal("An error occured")
	}
}
