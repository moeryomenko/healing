package checkers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moeryomenko/healing"
)

func TestIntegration_SQLReadiness(t *testing.T) {
	// start mysql.
	my := mysql.Preset(
		mysql.WithUser(user, password),
		mysql.WithDatabase(database),
		mysql.WithVersion("5.7.32"),
	)
	container, err := gnomock.Start(my)
	require.NoError(t, err)
	defer func() { gnomock.Stop(container) }()

	// create our mysql connections pool.
	dsn := fmt.Sprintf("%s:%s@(%s:%d)/%s?sql_mode=TRADITIONAL&parseTime=true&tls=false",
		user, password,
		container.Host, container.DefaultPort(),
		database,
	)
	pool, err := sql.Open(`mysql`, dsn)
	require.NoError(t, err)
	pool.SetMaxOpenConns(20)
	pool.SetMaxIdleConns(20)

	healthController := healing.New(8080)
	healthController.AddReadyChecker("mysql_controller", SQLPoolReadinessChecker(pool))

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go RunSQLLoad(workloadCtx, t, pool)
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

func RunSQLLoad(ctx context.Context, t *testing.T, pool *sql.DB) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for i := 0; i < 20; i++ {
				go startSQLIdleXact(ctx, pool)
			}
		}
	}
}

// startSQLIdleXact starts transaction and goes sleeping for specified amount of time.
func startSQLIdleXact(ctx context.Context, pool *sql.DB) {
	conn, err := pool.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault})
	if err != nil {
		return
	}
	defer func() { tx.Rollback() }()

	// Create a temp table using single row from target table. Later,
	// transaction will be rolled back and temp table will be dropped. Also, any errors could
	// be ignored, because in this case transaction (aborted) also stay idle.
	temp := time.Now().Unix()
	q := fmt.Sprintf("CREATE TABLE temp_%d (c INT)", temp)
	_, err = tx.Exec(q)
	if err != nil {
		return
	}

	// Stop execution only if context has been done or naptime interval is timed out.
	<-ctx.Done()
	return
}
