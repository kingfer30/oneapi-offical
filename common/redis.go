package common

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/songquanpeng/one-api/common/logger"
)

var RDB *redis.Client
var RedisEnabled = true

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {
	if os.Getenv("REDIS_CONN_STRING") == "" {
		RedisEnabled = false
		logger.SysLog("REDIS_CONN_STRING not set, Redis is not enabled")
		return nil
	}
	if os.Getenv("SYNC_FREQUENCY") == "" {
		RedisEnabled = false
		logger.SysLog("SYNC_FREQUENCY not set, Redis is disabled")
		return nil
	}
	logger.SysLog("Redis is enabled")
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		logger.FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	RDB = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = RDB.Ping(ctx).Result()
	if err != nil {
		logger.FatalLog("Redis ping test failed: " + err.Error())
	}
	return err
}

func ParseRedisOption() *redis.Options {
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		logger.FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	return opt
}

func RedisSet(key string, value string, expiration time.Duration) error {
	ctx := context.Background()
	return RDB.Set(ctx, key, value, expiration).Err()
}

func RedisGet(key string) (string, error) {
	ctx := context.Background()
	return RDB.Get(ctx, key).Result()
}

func RedisDel(key string) error {
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

func RedisDecrease(key string, value int64) error {
	ctx := context.Background()
	return RDB.DecrBy(ctx, key, value).Err()
}
func RedisExists(key string) (int64, error) {
	ctx := context.Background()
	return RDB.Exists(ctx, key).Result()
}

func RedisSetNx(key string, value string, expiration time.Duration) (bool, error) {
	ctx := context.Background()
	return RDB.SetNX(ctx, key, value, expiration).Result()
}

func RedisHashSet(key string, field string, value any, expiration int64) error {
	ctx := context.Background()
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = RDB.HSet(ctx, key, field, string(jsonBytes)).Result()
	if err != nil {
		return err
	}
	if expiration != 0 {
		expireTime := time.Duration(expiration) * time.Second
		err = RDB.Expire(ctx, key, expireTime).Err()
	}
	return err
}

func RedisHashGet(key string, field string) (string, error) {
	ctx := context.Background()
	return RDB.HGet(ctx, key, field).Result()
}

func RedisHashDel(key string, field string) error {
	ctx := context.Background()
	return RDB.HDel(ctx, key, field).Err()
}
