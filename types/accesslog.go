package types

import "time"

// AccesslogProduceLogArgs : Just a copy of accesslog.Log, to prevent import the "unsafe" package
type AccesslogProduceLogArgs struct {
	KeyID     *string
	Path      string
	Status    int
	Timestamp time.Time
}
