package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func New(dsn string) (*Client, error) {
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, err
	}

	opts.PoolSize = 10
	opts.MinIdleConns = 5
	opts.ConnMaxIdleTime = 5 * time.Minute
	opts.ConnMaxLifetime = 30 * time.Minute

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) RDB() *redis.Client {
	return c.rdb
}

// Cache helpers
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

// Rate limiting helpers
func (c *Client) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	pipe := c.rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, expiration)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (c *Client) GetInt(ctx context.Context, key string) (int64, error) {
	return c.rdb.Get(ctx, key).Int64()
}

