// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package metabase

import (
	"bytes"
	"context"
	"strings"

	"github.com/zeebo/errs"

	"storj.io/common/uuid"
	"storj.io/storj/private/tagsql"
)

// objectIterator enables iteration on objects in a bucket.
type objectsIterator struct {
	db *DB

	projectID  uuid.UUID
	bucketName string
	status     ObjectStatus
	limitKey   ObjectKey
	batchSize  int
	recursive  bool

	curIndex int
	curRows  tagsql.Rows
	cursor   IterateCursor

	skipPrefix ObjectKey
}

func iterateAllVersionsWithStatus(ctx context.Context, db *DB, opts IterateObjectsWithStatus, fn func(context.Context, ObjectsIterator) error) (err error) {
	defer mon.Task()(&ctx)(&err)

	it := &objectsIterator{
		db: db,

		projectID:  opts.ProjectID,
		bucketName: opts.BucketName,
		status:     opts.Status,
		limitKey:   nextPrefix(opts.Prefix),
		batchSize:  opts.BatchSize,
		recursive:  opts.Recursive,

		curIndex: 0,
		cursor:   opts.Cursor,
	}

	// ensure batch size is reasonable
	if it.batchSize <= 0 || it.batchSize > batchsizeLimit {
		it.batchSize = batchsizeLimit
	}

	// start from either the cursor or prefix, depending on which is larger
	if lessKey(it.cursor.Key, opts.Prefix) {
		it.cursor.Key = beforeKey(opts.Prefix)
		it.cursor.Version = -1
	}

	it.curRows, err = it.doNextQuery(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if rowsErr := it.curRows.Err(); rowsErr != nil {
			err = errs.Combine(err, rowsErr)
		}
		err = errs.Combine(err, it.curRows.Close())
	}()

	return fn(ctx, it)
}

// Next returns true if there was another item and copy it in item.
func (it *objectsIterator) Next(ctx context.Context, item *ObjectEntry) bool {
	if it.recursive {
		return it.next(ctx, item)
	}

	// TODO: implement this on the database side

	ok := it.next(ctx, item)
	if !ok {
		return false
	}

	// skip until we are past the prefix we returned before.
	if it.skipPrefix != "" {
		for strings.HasPrefix(string(item.ObjectKey), string(it.skipPrefix)) {
			if !it.next(ctx, item) {
				return false
			}
		}
		it.skipPrefix = ""
	}

	// should this be treated as a prefix?
	p := strings.IndexByte(string(item.ObjectKey), Delimiter)
	if p >= 0 {
		it.skipPrefix = item.ObjectKey[:p+1]
		*item = ObjectEntry{
			IsPrefix:  true,
			ObjectKey: item.ObjectKey[:p+1],
			Status:    it.status,
		}
	}

	return true
}

// next returns true if there was another item and copy it in item.
func (it *objectsIterator) next(ctx context.Context, item *ObjectEntry) bool {
	next := it.curRows.Next()
	if !next {
		if it.curIndex < it.batchSize {
			return false
		}

		if it.curRows.Err() != nil {
			return false
		}

		rows, err := it.doNextQuery(ctx)
		if err != nil {
			return false
		}

		if it.curRows.Close() != nil {
			_ = rows.Close()
			return false
		}

		it.curRows = rows
		it.curIndex = 0
		if !it.curRows.Next() {
			return false
		}
	}

	err := it.scanItem(item)
	if err != nil {
		return false
	}

	it.curIndex++
	it.cursor.Key = item.ObjectKey
	it.cursor.Version = item.Version

	return true
}

// doNextQuery executes query to fetch the next batch returning the rows.
func (it *objectsIterator) doNextQuery(ctx context.Context) (_ tagsql.Rows, err error) {
	defer mon.Task()(&ctx)(&err)

	if it.limitKey == "" {
		return it.db.db.Query(ctx, `
			SELECT
				object_key, stream_id, version, status,
				created_at, expires_at,
				segment_count,
				encrypted_metadata_nonce, encrypted_metadata, encrypted_metadata_encrypted_key,
				total_plain_size, total_encrypted_size, fixed_segment_size,
				encryption
			FROM objects
			WHERE
				project_id = $1 AND bucket_name = $2
				AND status = $3
				AND (object_key, version) > ($4, $5)
				ORDER BY object_key ASC, version ASC
			LIMIT $6
			`, it.projectID, it.bucketName,
			it.status,
			[]byte(it.cursor.Key), int(it.cursor.Version),
			it.batchSize,
		)
	}

	// TODO this query should use SUBSTRING(object_key from $8) but there is a problem how it
	// works with CRDB.
	return it.db.db.Query(ctx, `
		SELECT
			object_key, stream_id, version, status,
			created_at, expires_at,
			segment_count,
			encrypted_metadata_nonce, encrypted_metadata, encrypted_metadata_encrypted_key,
			total_plain_size, total_encrypted_size, fixed_segment_size,
			encryption
		FROM objects
		WHERE
			project_id = $1 AND bucket_name = $2
			AND status = $3
			AND (object_key, version) > ($4, $5)
			AND object_key < $6
			ORDER BY object_key ASC, version ASC
		LIMIT $7
	`, it.projectID, it.bucketName,
		it.status,
		[]byte(it.cursor.Key), int(it.cursor.Version),
		[]byte(it.limitKey),
		it.batchSize,

		// len(it.limitKey)+1, // TODO uncomment when CRDB issue will be fixed
	)
}

// scanItem scans doNextQuery results into ObjectEntry.
func (it *objectsIterator) scanItem(item *ObjectEntry) error {
	item.IsPrefix = false
	err := it.curRows.Scan(
		&item.ObjectKey, &item.StreamID, &item.Version, &item.Status,
		&item.CreatedAt, &item.ExpiresAt,
		&item.SegmentCount,
		&item.EncryptedMetadataNonce, &item.EncryptedMetadata, &item.EncryptedMetadataEncryptedKey,
		&item.TotalPlainSize, &item.TotalEncryptedSize, &item.FixedSegmentSize,
		encryptionParameters{&item.Encryption},
	)
	if err != nil {
		return err
	}

	// TODO this should be done with SQL query
	item.ObjectKey = item.ObjectKey[len(it.limitKey):]
	return nil
}

// nextPrefix returns the next prefix of the same length.
func nextPrefix(key ObjectKey) ObjectKey {
	if key == "" {
		return ""
	}
	after := []byte(key)
	after[len(after)-1]++
	return ObjectKey(after)
}

// beforeKey returns the key just before the key.
func beforeKey(key ObjectKey) ObjectKey {
	if key == "" {
		return ""
	}

	before := []byte(key)
	before[len(before)-1]--
	return ObjectKey(append(before, 0xFF))
}

// lessKey returns whether a < b.
func lessKey(a, b ObjectKey) bool {
	return bytes.Compare([]byte(a), []byte(b)) < 0
}
