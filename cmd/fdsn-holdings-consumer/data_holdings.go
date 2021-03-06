package main

import (
	"github.com/GeoNet/fdsn/internal/holdings"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/lib/pq"
)

// http://www.postgresql.org/docs/9.4/static/errcodes-appendix.html
const (
	errorUniqueViolation pq.ErrorCode = "23505"
)

type holding struct {
	holdings.Holding
	key string // the S3 bucket key
}

func (h *holding) save() error {
	r, err := h.saveHoldings()
	switch {
	case err != nil:
		return err
	case r == 1:
		return nil
	}

	_, err = h.saveStream()
	if err != nil {
		return err
	}

	_, err = h.saveHoldings()
	if err != nil {
		return err
	}

	return nil
}

func (h *holding) saveHoldings() (int64, error) {
	txn, err := db.Begin()

	_, err = txn.Exec(`DELETE FROM fdsn.holdings WHERE key=$1`, h.key)
	if err != nil {
		txn.Rollback()
		return 0, err
	}

	r, err := txn.Exec(`INSERT INTO fdsn.holdings (streamPK, start_time, numsamples, key)
	SELECT streamPK, $5, $6, $7
	FROM fdsn.stream
	WHERE network = $1
	AND station = $2
	AND channel = $3
	AND location = $4`, h.Network, h.Station, h.Channel, h.Location, h.Start,
		h.NumSamples, h.key)
	if err != nil {
		txn.Rollback()
		return 0, err
	}

	err = txn.Commit()
	if err != nil {
		return 0, err
	}

	return r.RowsAffected()
}

func (h *holding) saveStream() (int64, error) {
	r, err := db.Exec(`INSERT INTO fdsn.stream (network, station, channel, location) VALUES($1, $2, $3, $4)`,
		h.Network, h.Station, h.Channel, h.Location)
	if err != nil {
		if u, ok := err.(*pq.Error); ok && u.Code == errorUniqueViolation {
			return 1, nil
		} else {
			return 0, err
		}
	}

	return r.RowsAffected()
}

func (h *holding) delete() error {
	_, err := db.Exec(`DELETE FROM fdsn.holdings WHERE key = $1`, h.key)
	return err
}

func holdingS3(bucket, key string) (holding, error) {
	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Key:    aws.String(key),
		Bucket: aws.String(bucket),
	})

	if err != nil {
		return holding{}, err
	}
	defer result.Body.Close()

	h, err := holdings.SingleStream(result.Body)
	if err != nil {
		return holding{}, err
	}

	return holding{key: key, Holding: h}, nil
}
