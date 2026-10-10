// Package backup exports all redis data to readable json and restores it
package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
)

// version is format version of backup file
const version = 1

// File is a backup, values are by key type: string, set and list are []string, hash is map[string]string
type File struct {
	Version int              `json:"version"`
	Created time.Time        `json:"created"`
	Keys    map[string]Value `json:"keys"`
}

// Value is a redis key
type Value struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// Export writes all keys as json
func Export(w io.Writer) error {
	keys, err := connections.Scan("*")
	if err != nil {
		return err
	}
	sort.Strings(keys)
	conn := connections.Redis()
	defer conn.Close()
	f := File{Version: version, Created: time.Now(), Keys: make(map[string]Value, len(keys))}
	for _, key := range keys {
		typ, err := redis.String(conn.Do("TYPE", key))
		if err != nil {
			return err
		}
		var v interface{}
		switch typ {
		case "string":
			v, err = redis.String(conn.Do("GET", key))
		case "set":
			var members []string
			members, err = redis.Strings(conn.Do("SMEMBERS", key))
			sort.Strings(members)
			v = members
		case "list":
			v, err = redis.Strings(conn.Do("LRANGE", key, 0, -1))
		case "hash":
			v, err = redis.StringMap(conn.Do("HGETALL", key))
		case "none":
			// deleted after scanning
			continue
		default:
			return fmt.Errorf("key %s: unsupported type %s", key, typ)
		}
		if err != nil {
			return fmt.Errorf("key %s: %w", key, err)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		f.Keys[key] = Value{Type: typ, Value: raw}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(f)
}

// Import replaces all data with backup, nothing changes when backup is invalid
func Import(r io.Reader) (keys int, err error) {
	var f File
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return 0, fmt.Errorf("invalid backup: %w", err)
	}
	if f.Version != version {
		return 0, fmt.Errorf("unsupported backup version %d", f.Version)
	}
	// decode all before writing
	args := make([]redis.Args, 0, len(f.Keys))
	for key, v := range f.Keys {
		var a redis.Args
		switch v.Type {
		case "string":
			var s string
			err = json.Unmarshal(v.Value, &s)
			a = redis.Args{"SET", key, s}
		case "set", "list":
			var items []string
			err = json.Unmarshal(v.Value, &items)
			cmd := "SADD"
			if v.Type == "list" {
				cmd = "RPUSH"
			}
			a = redis.Args{cmd, key}.AddFlat(items)
		case "hash":
			var m map[string]string
			err = json.Unmarshal(v.Value, &m)
			a = redis.Args{"HSET", key}.AddFlat(m)
		default:
			err = fmt.Errorf("unsupported type %s", v.Type)
		}
		if err != nil {
			return 0, fmt.Errorf("key %s: %w", key, err)
		}
		// empty set, list or hash does not exist in redis
		if len(a) > 2 {
			args = append(args, a)
		}
	}

	conn := connections.Redis()
	defer conn.Close()
	conn.Send("MULTI")
	conn.Send("FLUSHDB")
	for _, a := range args {
		conn.Send(a[0].(string), a[1:]...)
	}
	if _, err := conn.Do("EXEC"); err != nil {
		return 0, err
	}
	return len(args), nil
}
