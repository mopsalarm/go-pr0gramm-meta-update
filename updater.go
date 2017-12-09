package main

import (
	"database/sql"
	"time"

	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/mopsalarm/go-pr0gramm"
	"github.com/rcrowley/go-metrics"
	"io/ioutil"
	"net/http"
	"strings"
	"unicode/utf8"
)

type tagWithItemId struct {
	pr0gramm.Tag
	ItemId pr0gramm.Id
}

func UpdateAll(db *sql.DB, request pr0gramm.ItemsRequest) (pr0gramm.Id, error) {
	var lastProcessedId pr0gramm.Id = request.Older
	err := pr0gramm.StreamPaged(request, func(items []pr0gramm.Item) (bool, error) {
		var deleteQueue []pr0gramm.Id
		for _, item := range items {
			if lastProcessedId > 0 {
				for id := lastProcessedId - 1; id > item.Id; id-- {
					deleteQueue = append(deleteQueue, id)
				}
			}

			lastProcessedId = item.Id
		}

		if len(deleteQueue) > 0 {
			deleteItems(db, deleteQueue...)
		}

		writeItems(db, items)
		time.Sleep(5 * time.Second)
		return true, nil
	})

	return lastProcessedId, err
}

func Update(db *sql.DB, maxItemAge time.Duration) {
	logrus.WithField("max-age", maxItemAge).Info("Updating items now")

	var lastItemId pr0gramm.Id

	var deleteQueue []pr0gramm.Id
	var updateQueue []pr0gramm.Item

	err := pr0gramm.Stream(pr0gramm.NewItemsRequest(), pr0gramm.ConsumeIf(
		func(item pr0gramm.Item) bool {
			if lastItemId > 0 {
				for id := lastItemId - 1; id > item.Id; id-- {
					deleteQueue = append(deleteQueue, id)
				}
			}

			lastItemId = item.Id

			return time.Since(item.Created.Time).Seconds() < maxItemAge.Seconds()
		},
		func(item pr0gramm.Item) error {
			updateQueue = append(updateQueue, item)
			return nil
		}))

	if err != nil {
		logrus.WithError(err).Warn("Could not fetch all items")
	}

	if len(deleteQueue) > 0 {
		logrus.Infof("Ensure that %d out of %d items are absent", len(deleteQueue), len(updateQueue))
		deleteItems(db, deleteQueue...)
	}

	if len(updateQueue) == 0 {
		return
	}

	writeItems(db, updateQueue)
}

func deleteItems(db *sql.DB, itemIds ...pr0gramm.Id) {
	tx, err := db.Begin()
	if err != nil {
		logrus.WithError(err).Warn("Could not open transction")
		return
	}

	defer tx.Commit()

	if res, err := tx.Exec("DELETE FROM items WHERE id=ANY($1)", pq.GenericArray{A: itemIds}); err != nil {
		logrus.Warnf("Could not delete %d items from database", len(itemIds))
		return
	} else {
		affected, _ := res.RowsAffected()
		logrus.Infof("%d items were deleted from database", affected)
	}
}

func writeItems(db *sql.DB, items []pr0gramm.Item) {
	tx, err := db.Begin()
	if err != nil {
		logrus.WithError(err).Warn("Could not open transction")
		return
	}

	defer tx.Commit()

	itemStmt, err := tx.Prepare(`INSERT INTO items
		(id, promoted, up, down, created, image, thumb, fullsize, source, flags, username, mark, width, height, audio, updated)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE
			SET up=EXCLUDED.up, down=EXCLUDED.down, promoted=EXCLUDED.promoted, mark=EXCLUDED.mark, updated=EXCLUDED.updated, flags=EXCLUDED.flags
		WHERE items.up!=EXCLUDED.up OR items.down!=EXCLUDED.down OR items.promoted!=EXCLUDED.promoted OR items.mark!=EXCLUDED.mark OR items.flags!=EXCLUDED.flags`)

	if err != nil {
		logrus.WithError(err).Warn("Could not prepare insert statement")
		return
	}

	defer itemStmt.Close()

	start := time.Now()
	logrus.WithField("count", len(items)).Info("Writing items to database")
	for _, item := range items {
		_, err := itemStmt.Exec(
			uint64(item.Id), uint64(item.Promoted),
			item.Up, item.Down,
			item.Created.Time.Unix(),
			item.Image, item.Thumbnail, item.Fullsize, item.Source,
			item.Flags, item.User, item.Mark, item.Width, item.Height, item.Audio,
			time.Now().UTC())

		if err != nil {
			logrus.WithError(err).Warn("Could not insert item into database, skipping.")
		} else {
			metrics.GetOrRegisterMeter("pr0gramm.meta.items.inserted", nil).Mark(1)
		}
	}

	logrus.
		WithField("duration", time.Since(start)).
		WithField("count", len(items)).
		Info("Finished writing items")
}

func UpdateTags(db *sql.DB) int {
	start := time.Now()

	tx, err := db.Begin()
	if err != nil {
		logrus.WithError(err).Error("Could not create transaction to update tags")
		return 0
	}

	defer tx.Commit()

	var largestTagId uint64

	row := tx.QueryRow("SELECT COALESCE(MAX(id), 0) FROM tags")
	if err := row.Scan(&largestTagId); err != nil {
		logrus.WithError(err).Error("Could not get the value of the largest known tag-id")
		return 0
	}

	url := fmt.Sprintf("http://pr0gramm.com/api/tags/latest?id=%d", largestTagId)
	logrus.WithField("url", url).Info("Fetching tags from remote api now")

	response, err := http.Get(url)
	if err != nil {
		logrus.WithError(err).Error("Request to get the latest tags failed.")
		return 0
	}

	defer func() {
		ioutil.ReadAll(response.Body)
		response.Body.Close()
	}()

	var decoded struct {
		Tags []struct {
			Id         uint64
			Up         int32
			Down       int32
			Confidence float32
			ItemId     uint64
			Tag        string
		}
	}

	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		logrus.WithError(err).Error("Could not decode response")
		return 0
	}

	if len(decoded.Tags) > 0 {
		tagStmt, err := tx.Prepare(`INSERT INTO tags
		(id, item_id, up, down, confidence, tag)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		SET up=EXCLUDED.up, down=EXCLUDED.down, confidence=EXCLUDED.confidence
		WHERE tags.up!=EXCLUDED.up OR tags.down!=EXCLUDED.down`)

		if err != nil {
			logrus.WithError(err).Error("Could not prepare statement")
			return 0
		}

		defer tagStmt.Close()

		logrus.WithField("count", len(decoded.Tags)).Info("Will dump tags to database.")
		for _, tag := range decoded.Tags {
			if utf8.ValidString(tag.Tag) && !strings.ContainsRune(tag.Tag, 0) {
				_, err = tagStmt.Exec(tag.Id, tag.ItemId, tag.Up, tag.Down, tag.Confidence, tag.Tag)

				if err != nil {
					logrus.WithError(err).Warn("Could not insert tag into database, skipping")
				} else {
					metrics.GetOrRegisterMeter("pr0gramm.meta.tags.inserted", nil).Mark(1)
				}
			}
		}
	}

	logrus.
		WithField("duration", time.Since(start)).
		WithField("count", len(decoded.Tags)).
		Info("Finished writing tags")

	return len(decoded.Tags)
}
