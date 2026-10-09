package connections

import (
	"os"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/garyburd/redigo/redis"
)

var pool = newPool()

func newPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 300 * time.Second,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", os.Getenv("REDIS_ENDPOINT")+":"+os.Getenv("REDIS_PORT"))
			if err != nil {
				log.WithError(err).Error("Redis Dial Failed")
			}
			return conn, err
		},
	}
}

// Redis get redis connection
func Redis() redis.Conn {
	return pool.Get()
}
