package pg

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification is a type alias of pgconn.Notification type.
type Notification = pgconn.Notification

// Listener represents a postgres database LISTEN connection.
type Listener struct {
	conn *pgxpool.Conn

	channel string
}

// ErrEmptyPayload is returned when the notification payload is empty.
var ErrEmptyPayload = fmt.Errorf("empty payload")

// Accept waits for a notification and returns it.
func (l *Listener) Accept(ctx context.Context) (*Notification, error) {
	nf, err := l.conn.Conn().WaitForNotification(ctx)
	if err != nil {
		return nil, err
	}

	/* Sadly this is not possible due to the Go's limitations.
	var payload T
	if s, ok := payload.(string); ok {
		// use nativeAccept.
	}
	*/

	if len(nf.Payload) == 0 {
		return nil, ErrEmptyPayload
	}

	return nf, nil
}

// Close closes the listener connection.
func (l *Listener) Close(ctx context.Context) error {
	if l.conn == nil {
		return nil
	}
	defer l.conn.Release()

	query := `SELECT UNLISTEN $1;`
	_, err := l.conn.Exec(ctx, query, l.channel)
	if err != nil {
		return err
	}

	return nil
}

// notifyJSON sends a notification of any type to the underline database listener.
func notifyJSON(ctx context.Context, db *DB, channel string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return notifyNative(ctx, db, channel, b)
}

// NotifyNative sends a raw notification to the underline database listener,
// it accepts string or a slice of bytes because that's the only raw types that are allowed to be delivered.
func notifyNative[T string | []byte](ctx context.Context, db *DB, channel string, payload T) error {
	query := `SELECT pg_notify($1, $2)`
	_, err := db.Pool.Exec(context.Background(), query, channel, payload) // Always on top.
	return err
}

// UnmarshalNotification returns the notification payload as a custom type of T.
func UnmarshalNotification[T any](n *Notification) (T, error) {
	var payload T

	b, err := stringToBytes(n.Payload)
	if err != nil {
		return payload, err
	}

	err = json.Unmarshal(b, &payload)
	if err != nil {
		return payload, err
	}

	return payload, nil
}

// based on: https://groups.google.com/g/golang-nuts/c/Zsfk-VMd_fU/m/O1ru4fO-BgAJ.
func stringToBytes(s string) ([]byte, error) {
	const max = 0x7fff0000
	if len(s) > max {
		return nil, fmt.Errorf("string too long")
	}

	return (*[max]byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data))[:len(s):len(s)], nil
}
