package backup

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gomodule/redigo/redis"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func do(t *testing.T, cmd string, args ...interface{}) interface{} {
	t.Helper()
	conn := connections.Redis()
	defer conn.Close()
	reply, err := conn.Do(cmd, args...)
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestExportImport(t *testing.T) {
	redistest.Flush()
	do(t, "SET", "user:1", `{"enable":true}`)
	do(t, "SADD", "boards", "stock", "movie")
	do(t, "RPUSH", "notify:recent", "b", "a")
	do(t, "HSET", "settings", "telegram_token", "x", "registration", "open")

	var buf bytes.Buffer
	if err := Export(&buf); err != nil {
		t.Fatal(err)
	}
	backup := buf.String()

	do(t, "SET", "user:2", "{}")
	do(t, "DEL", "boards")
	n, err := Import(strings.NewReader(backup))
	if err != nil || n != 4 {
		t.Fatalf("Import() = %d, %v", n, err)
	}

	if s, _ := redis.String(do(t, "GET", "user:1"), nil); s != `{"enable":true}` {
		t.Errorf("user:1 = %q", s)
	}
	if n, _ := redis.Int(do(t, "EXISTS", "user:2"), nil); n != 0 {
		t.Error("keys not in backup should be removed")
	}
	if boards, _ := redis.Strings(do(t, "SMEMBERS", "boards"), nil); len(boards) != 2 {
		t.Errorf("boards = %v", boards)
	}
	if list, _ := redis.Strings(do(t, "LRANGE", "notify:recent", 0, -1), nil); !reflect.DeepEqual(list, []string{"b", "a"}) {
		t.Errorf("notify:recent = %v", list)
	}
	if m, _ := redis.StringMap(do(t, "HGETALL", "settings"), nil); m["registration"] != "open" || m["telegram_token"] != "x" {
		t.Errorf("settings = %v", m)
	}

	buf.Reset()
	if err := Export(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"boards"`) {
		t.Error("export after import should contain the same keys")
	}
}

func TestImportInvalidKeepsData(t *testing.T) {
	redistest.Flush()
	do(t, "SET", "user:1", "{}")
	for _, backup := range []string{
		"not json",
		`{"version":99,"keys":{}}`,
		`{"version":1,"keys":{"a":{"type":"zset","value":[]}}}`,
		`{"version":1,"keys":{"a":{"type":"set","value":"x"}}}`,
	} {
		if _, err := Import(strings.NewReader(backup)); err == nil {
			t.Errorf("Import(%s) should fail", backup)
		}
	}
	if n, _ := redis.Int(do(t, "EXISTS", "user:1"), nil); n != 1 {
		t.Error("failed import should keep data")
	}
}
