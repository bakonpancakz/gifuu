package tools

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	SNOWFLAKE_MAX_MACHINE_ID int64 = (1 << 10) - 1
	SNOWFLAKE_MAX_SEQUENCE   int64 = (1 << 12) - 1
	mtx                      sync.Mutex
	machineID                int64
	sequence                 int64
	timestamp                int64
)

type Snowflake int64

func (d *Snowflake) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*d = Snowflake(v)
	return nil
}

func init() {
	if v, err := strconv.ParseInt(MACHINE_ID, 10, 64); err != nil {
		panic("machine id is invalid")
	} else {
		machineID = v
		if machineID > SNOWFLAKE_MAX_MACHINE_ID {
			panic("machine id must fit within 10 bits")
		}
	}
}

// Generate a unique snowflake from this machine
func GenerateSnowflake() int64 {
	mtx.Lock()
	defer mtx.Unlock()

	now := time.Now().UnixMilli()

	if now != timestamp {
		sequence = 0
	} else {
		sequence++
		if sequence > SNOWFLAKE_MAX_SEQUENCE {
			for now <= timestamp {
				time.Sleep(time.Millisecond)
				now = time.Now().UnixMilli()
			}
			sequence = 0
		}
	}

	timestamp = now
	return ((now - EPOCH_MILLI) << 22) | (machineID << 12) | sequence
}
