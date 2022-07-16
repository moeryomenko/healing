package checkers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/go-mysql-org/go-mysql/client"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moeryomenko/healing"
)

// db settings.
const (
	user     = "test"
	password = "testpass"
	database = "testdb"
)

func TestIntegration_MySQLReadiness(t *testing.T) {
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
	pool := client.NewPool(
		log.Default().Printf,
		2,
		2,
		2,
		fmt.Sprintf("%s:%d", container.Host, container.DefaultPort()),
		user, password,
		database,
	)

	healthController := healing.New(8080)
	healthController.AddReadyChecker("mysql_controller", MySQLReadinessProber(pool))

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go RunMySQLLoad(workloadCtx, t, pool)
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

func RunMySQLLoad(ctx context.Context, t *testing.T, pool *client.Pool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for i := 0; i < 100; i++ {
				go startMySQLIdleXact(ctx, pool)
			}
		}
	}
}

// startMySQLIdleXact starts transaction and goes sleeping for specified amount of time.
func startMySQLIdleXact(ctx context.Context, pool *client.Pool) {
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
	q := fmt.Sprintf("CREATE TABLE temp_%d (c INT)", temp)
	_, err = conn.Execute(q)
	if err != nil {
		return
	}

	// Stop execution only if context has been done or naptime interval is timed out.
	<-ctx.Done()
	return
}
