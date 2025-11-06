package types

import (
    "time"
)

type Source struct {
    SourceID      string
    Name          string
    Token         string
    Enabled       bool
    IPAllowCIDRs  []string
    MaxBodyBytes  int32
    CreatedAt     time.Time
}

type Destination struct {
    DestinationID string
    Name          string
    URL           string
    HeadersJSON   []byte
    Secret        *string
    ConnectTimeoutS int32
    TimeoutS      int32
    VerifyTLS     bool

    MaxRPS        float64
    Burst         int32
    MaxInflight   int32

    BreakerFailureRatio float64
    BreakerMinRequests  int32
    BreakerCooldownS    int32

    CreatedAt     time.Time
}

type Route struct {
    RouteID       string
    SourceID      string
    DestinationID string
    Enabled       bool
    ContentTypeLike *string
    JSONPath      *string
    JSONEquals    *string
    RateOverrideJSON []byte
}

type Event struct {
    EventID      string
    SourceID     string
    ReceivedAt   time.Time
    ContentType  *string
    HeadersJSON  []byte
    Body         []byte
    BodySize     int32
    SourceIP     *string
    IdempotencyKey *string
    BodyHashSHA256 []byte
}

type Attempt struct {
    AttemptID   string
    EventID     string
    RouteID     string
    AttemptNo   int32
    Status      string
}

