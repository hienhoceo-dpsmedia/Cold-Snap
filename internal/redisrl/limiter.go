package redisrl

import (
    "context"
    "time"

    "github.com/redis/go-redis/v9"
)

const script = `
local rl = KEYS[1]; local infl = KEYS[2]
local now = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local rps = tonumber(ARGV[3])
local max_inflight = tonumber(ARGV[4])

local t = redis.call('HMGET', rl, 'tokens', 'ts')
local tokens = tonumber(t[1]) or burst
local ts = tonumber(t[2]) or now
local delta = math.max(0, now - ts)
local refill = delta * rps / 1000.0
tokens = math.min(burst, tokens + refill)

local inflight = tonumber(redis.call('GET', infl)) or 0

if inflight >= max_inflight then
  return {0, 100}
end

if tokens >= 1.0 then
  tokens = tokens - 1.0
  redis.call('HMSET', rl, 'tokens', tokens, 'ts', now)
  redis.call('PEXPIRE', rl, 60000)
  redis.call('INCR', infl)
  redis.call('PEXPIRE', infl, 60000)
  return {1, 0}
else
  local need = 1.0 - tokens
  local wait_ms = math.ceil(1000.0 * need / rps)
  redis.call('HMSET', rl, 'tokens', tokens, 'ts', now)
  redis.call('PEXPIRE', rl, 60000)
  return {0, wait_ms}
end
`

type Limiter struct {
    Rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter { return &Limiter{Rdb: rdb} }

// Allow tries to consume a token and increment inflight if permitted.
// Returns allowed, waitMs, error.
func (l *Limiter) Allow(ctx context.Context, destinationID string, burst int, rps float64, maxInflight int) (bool, int64, error) {
    rlKey := "rl:" + destinationID
    ifKey := "if:" + destinationID
    now := time.Now().UnixMilli()
    res, err := l.Rdb.Eval(ctx, script, []string{rlKey, ifKey}, now, burst, rps, maxInflight).Result()
    if err != nil {
        return false, 0, err
    }
    arr := res.([]interface{})
    allowed := arr[0].(int64) == 1
    wait := arr[1].(int64)
    return allowed, wait, nil
}

func (l *Limiter) Done(ctx context.Context, destinationID string) {
    ifKey := "if:" + destinationID
    l.Rdb.Watch(ctx, func(tx *redis.Tx) error {
        n, err := tx.Get(ctx, ifKey).Int64()
        if err != nil && err != redis.Nil { return err }
        if n <= 0 { _ = tx.Del(ctx, ifKey).Err(); return nil }
        pipe := tx.TxPipeline()
        pipe.Decr(ctx, ifKey)
        _, _ = pipe.Exec(ctx)
        return nil
    }, ifKey)
}

