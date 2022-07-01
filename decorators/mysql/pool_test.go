package mysql

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/moeryomenko/healing"
	"github.com/moeryomenko/squad"

	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pg settings.
const (
	user     = "test"
	password = "testpass"
	database = "testdb"
)

func TestIntegration_Liveness(t *testing.T) {
	// start postgresql.
	my := mysql.Preset(
		mysql.WithUser(user, password),
		mysql.WithDatabase(database),
		mysql.WithVersion("5.7.32"),
	)
	container, err := gnomock.Start(my)
	require.NoError(t, err)
	defer func() { gnomock.Stop(container) }()

	// create our pg connections pool.
	mypool, err := New(context.Background(), Config{
		User:     user,
		Password: password,
		Host:     container.Host,
		Port:     uint16(container.DefaultPort()),
		DBName:   database,
	},
		WithPoolConfig(PoolConfig{
			MinAlive: 2,
			MaxAlive: 2,
			MaxIdle:  2,
		}),
	)
	require.NoError(t, err)

	healthController := healing.New(healing.WithCheckPeriod(100 * time.Millisecond))
	healthController.AddReadyChecker("mysql_controller", mypool.CheckReadinessProber)

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go Run(squad.WithDelay(workloadCtx, 3*time.Second), t, mypool)
	// run readiness controller.
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer healthCancel()
	go healthController.Heartbeat(healthCtx)
	go func() {
		healthController.ListenAndServe(8080)(context.Background())
	}()

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

func Run(ctx context.Context, t *testing.T, pool *Pool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			go func() { startSingleIdleXact(ctx, pool) }()
		}
	}
}

// startSingleIdleXact starts transaction and goes sleeping for specified amount of time.
func startSingleIdleXact(ctx context.Context, pool *Pool) {
	conn, err := pool.GetConn(ctx)
	if err != nil {
		return
	}
	defer pool.PutConn(conn)
	err = conn.Begin()
	if err != nil {
		return
	}
	defer func() { _ = conn.Rollback() }()

	// Create a temp table using single row from target table. Later,
	// transaction will be rolled back and temp table will be dropped. Also, any errors could
	// be ignored, because in this case transaction (aborted) also stay idle.
	temp := time.Now().Unix()
	q := fmt.Sprintf("CREATE TEMPORARY TABLE temp_%d (c INT)", temp)
	_, err = conn.Execute(q)
	if err != nil {
		return
	}

	// Stop execution only if context has been done.
	<-ctx.Done()
	return
}
