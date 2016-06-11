package main

import (
	"database/sql"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mopsalarm/go-pr0gramm"
)

func Update(db *sql.DB, maxItemAge time.Duration) {
	logrus.WithField("max-age", maxItemAge).Info("Updating items now")

	var items []pr0gramm.Item
	err := pr0gramm.Stream(pr0gramm.NewItemsRequest(), pr0gramm.ConsumeIf(
		func(item pr0gramm.Item) bool {
			return time.Since(item.Created.Time).Seconds() < maxItemAge.Seconds()
		},
		func(item pr0gramm.Item) error {
			items = append(items, item)
			return nil
		}))

	if err != nil {
		logrus.WithError(err).Warn("Could not fetch all items");
	}

	if len(items) == 0 {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		logrus.WithError(err).Warn("Could not open transction")
		return
	}

	defer tx.Commit()

	statement, err := tx.Prepare(`INSERT INTO items
		(id, promoted,up, down, created, image, thumb, fullsize, source, flags, username, mark, width, height, audio)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (id) DO UPDATE SET up=EXCLUDED.up, down=EXCLUDED.down`)

	if err != nil {
		logrus.WithError(err).Warn("Could not prepare insert statement")
		return
	}

	defer statement.Close()

	start := time.Now()
	logrus.WithField("count", len(items)).Info("Writing items to database")
	for _, item := range items {
		_, err := statement.Exec(
			uint64(item.Id), uint64(item.Promoted),
			item.Up, item.Down,
			item.Created.Time.Unix(),
			item.Image, item.Thumbnail, item.Fullsize, item.Source,
			item.Flags, item.User, item.Mark, item.Width, item.Height, item.Audio)

		if err != nil {
			logrus.WithError(err).Warn("Could not insert item into database, skipping.")
		}
	}

	logrus.
	WithField("duration", time.Since(start)).
		WithField("count", len(items)).
		Info("Finished writing items")
}
