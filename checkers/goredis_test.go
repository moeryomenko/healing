package checkers

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/redis"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moeryomenko/healing"
)

func TestIntegration_RedisReadiness(t *testing.T) {
	// start mysql.
	r := redis.Preset()
	container, err := gnomock.Start(r)
	require.NoError(t, err)
	defer func() { gnomock.Stop(container) }()

	// create our redis connections pool.
	client := redisclient.NewClient(&redisclient.Options{Addr: container.DefaultAddress()})

	healthController := healing.New(8080)
	healthController.AddReadyChecker("mysql_controller", RedisReadinessProber(client))

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go RunRedisLoad(workloadCtx, t, client)
	// run readiness controller.
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer healthCancel()
	go healthController.Heartbeat(healthCtx)
	defer healthController.Stop(context.Background())

	<-time.After(time.Second)

	readinessTicker := time.NewTicker(time.Second)
	defer readinessTicker.Stop()
	loadIsStopped := false
	for {
		select {
		case <-workloadCtx.Done():
			loadIsStopped = true
		case <-readinessTicker.C:
			resp, err := http.Get("http://localhost:8080/ready")
			assert.NoError(t, err)
			if loadIsStopped {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			} else {
				assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			}
		case <-healthCtx.Done():
			return
		}
	}
}

func RunRedisLoad(ctx context.Context, t *testing.T, pool *redisclient.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("key_%d", i)
				pool.Pipelined(ctx, func(p redisclient.Pipeliner) error {
					for value := 0; value < 10; value++ {
						p.SAdd(ctx, key, time.Now().UTC())
					}
					<-ctx.Done()
					p.Del(ctx, key)
					return nil
				})
			}
		}
	}
}
