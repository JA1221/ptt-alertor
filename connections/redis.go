package connections

import (
	"os"
	"time"

	"github.com/gomodule/redigo/redis"
)

var pool = &redis.Pool{
	MaxIdle:     5,
	IdleTimeout: 300 * time.Second,
	Dial: func() (redis.Conn, error) {
		return redis.Dial("tcp", RedisAddr(),
			redis.DialConnectTimeout(5*time.Second),
			redis.DialReadTimeout(10*time.Second),
			redis.DialWriteTimeout(10*time.Second))
	},
	TestOnBorrow: func(c redis.Conn, t time.Time) error {
		if time.Since(t) < time.Minute {
			return nil
		}
		_, err := c.Do("PING")
		return err
	},
}

// RedisAddr returns REDIS_ENDPOINT:REDIS_PORT, default localhost:6379
func RedisAddr() string {
	host, port := os.Getenv("REDIS_ENDPOINT"), os.Getenv("REDIS_PORT")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "6379"
	}
	return host + ":" + port
}

// Redis get redis connection
func Redis() redis.Conn {
	return pool.Get()
}

// PingRedis checks redis is reachable
func PingRedis() error {
	conn := Redis()
	defer conn.Close()
	_, err := conn.Do("PING")
	return err
}
