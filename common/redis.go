package common

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/songquanpeng/one-api/common/logger"
)

var RDB redis.Cmdable
var RedisEnabled = true

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {
	if os.Getenv("REDIS_CONN_STRING") == "" {
		RedisEnabled = false
		logger.SysLog("REDIS_CONN_STRING not set, Redis is not enabled")
		return nil
	}
	redisConnString := os.Getenv("REDIS_CONN_STRING")
	if os.Getenv("REDIS_MASTER_NAME") == "" {
		logger.SysLog("Redis is enabled")
		opt, err := redis.ParseURL(redisConnString)
		if err != nil {
			logger.FatalLog("failed to parse Redis connection string: " + err.Error())
		}
		RDB = redis.NewClient(opt)
	} else {
		// cluster mode
		logger.SysLog("Redis cluster mode enabled")
		RDB = redis.NewUniversalClient(&redis.UniversalOptions{
			Addrs:      strings.Split(redisConnString, ","),
			Password:   os.Getenv("REDIS_PASSWORD"),
			MasterName: os.Getenv("REDIS_MASTER_NAME"),
		})
	}
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

func RedisRpush(key string, value string) error {
	ctx := context.Background()
	return RDB.RPush(ctx, key, value).Err()
}

func RedisLPop(key string) (string, error) {
	ctx := context.Background()
	return RDB.LPop(ctx, key).Result()
}

func RedisSadd(key string, value string, expiration time.Duration) error {
	ctx := context.Background()
	_, err := RDB.SAdd(ctx, key, value, expiration).Result()
	if err != nil {
		return err
	}
	return RDB.Expire(ctx, key, expiration).Err()
}
func RedisZadd(key string, value string, score float64, expiration time.Duration) error {
	ctx := context.Background()
	_, err := RDB.ZAdd(ctx, key, &redis.Z{
		Score:  score,
		Member: value,
	}).Result()
	if err != nil {
		return err
	}
	return RDB.Expire(ctx, key, expiration).Err()
}
func RedisZdel(key string, value []string) (int64, error) {
	ctx := context.Background()
	return RDB.ZRem(ctx, key, value).Result()
}
func RedisZRangeByScore(key string, count int, opt *redis.ZRangeBy) ([]string, error) {
	ctx := context.Background()
	return RDB.ZRangeByScore(ctx, key, opt).Result()
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
