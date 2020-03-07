package component

import (
	"encoding/base32"
	"time"

	"golang.org/x/crypto/blake2b"
)

type LogEvent struct {
	Partition string
	Timestamp time.Time
	Message   string
}

// An interface returned by a platforms logging system that returns
// batches of log lines
type LogViewer interface {
	NextBatch() ([]LogEvent, error)
}

type PartitionViewer struct {
	shortened map[string]string
}

var encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567")

func (pv *PartitionViewer) Short(part string) string {
	if len(part) < 10 {
		return part
	}

	if pv.shortened == nil {
		pv.shortened = make(map[string]string)
	}

	if short, ok := pv.shortened[part]; ok {
		return short
	}

	h, _ := blake2b.New(blake2b.Size, nil)

	h.Write([]byte(part))

	str := encoding.EncodeToString(h.Sum(nil))

	length := 7

	for {
		short := str[:length]
		if _, found := pv.shortened[short]; found {
			length++
		} else {
			pv.shortened[part] = short
			return short
		}
	}
}