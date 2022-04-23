package mysql

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/go-mysql-org/go-mysql/client"
	"github.com/moeryomenko/healing"

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

	// LocktimeMin defines a lower threshold of locking interval for blocking transactions.
	LocktimeMin time.Duration = time.Second
	// LocktimeMax defines an upper threshold of locking interval for blocking transactions.
	LocktimeMax time.Duration = 2 * time.Second
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
	)
	require.NoError(t, err)

	healthController := healing.New()
	healthController.AddReadyChecker("mysql_controller", mypool.CheckReadinessProber)

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go Run(workloadCtx, t, mypool)
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
	// Initialize random, used for calculating lock duration.
	rand.Seed(time.Now().UnixNano())

	// defaultConnInterval defines default interval between making new connection to Postgres
	defaultConnInterval := 50 * time.Millisecond

	interval := defaultConnInterval
	timer := time.NewTimer(interval)

	// Increment maxTime up to 1 second due to rand.Int63n() never return max value.
	minTime, maxTime := LocktimeMin, LocktimeMax+1

	for {
		select {
		// run workers only when it's possible to write into channel (channel is limited by number of jobs)
		case <-timer.C:
			naptime := time.Duration(rand.Int63n(maxTime.Nanoseconds()-minTime.Nanoseconds()) + minTime.Nanoseconds())
			err := startSingleIdleXact(ctx, pool, naptime)
			if err != nil {
				t.Log(err.Error())
				// if connect has failed, increase interval between connects.
				interval *= 2
			} else {
				// if attempt was successful reduce interval, but no less than default.
				if interval > defaultConnInterval {
					interval /= 2
				}
			}

			timer.Reset(interval)
		case <-ctx.Done():
			return
		}
	}
}

// startSingleIdleXact starts transaction and goes sleeping for specified amount of time.
func startSingleIdleXact(ctx context.Context, pool *Pool, naptime time.Duration) error {
	conn, err := pool.GetConn(ctx)
	if err != nil {
		return err
	}
	defer pool.PutConn(conn)
	err = conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Rollback() }()

	// Create a temp table using single row from target table. Later,
	// transaction will be rolled back and temp table will be dropped. Also, any errors could
	// be ignored, because in this case transaction (aborted) also stay idle.
	err = createTempTable(conn)
	if err != nil {
		return err
	}

	// Stop execution only if context has been done or naptime interval is timed out.
	timer := time.NewTimer(naptime)
	select {
	case <-ctx.Done():
		return nil
	case <-timer.C:
		return nil
	}
}

// createTempTable creates a temporary table within a transaction using single row from temp table.
func createTempTable(tx *client.Conn) error {
	temp := time.Now().Unix()
	q := fmt.Sprintf("CREATE TEMPORARY TABLE temp_%d (c INT)", temp)
	_, err := tx.Execute(q)
	if err != nil {
		return err
	}

	return nil
}
