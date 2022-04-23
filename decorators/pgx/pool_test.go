package pgx

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/moeryomenko/healing"

	"github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pg settings.
const (
	user     = "test"
	password = "testpass"
	database = "testdb"
	dsn      = "host=%s port=%d user=%s password=%s database=%s"

	// LocktimeMin defines a lower threshold of locking interval for blocking transactions.
	LocktimeMin time.Duration = time.Second
	// LocktimeMax defines an upper threshold of locking interval for blocking transactions.
	LocktimeMax time.Duration = 2 * time.Second
)

func TestIntegration_Liveness(t *testing.T) {
	// start postgresql.
	pg := postgres.Preset(
		postgres.WithUser(user, password),
		postgres.WithDatabase(database),
	)
	container, err := gnomock.Start(pg)
	require.NoError(t, err)
	defer func() { gnomock.Stop(container) }()

	// create our pg connections pool.
	pgpool, err := New(context.Background(), Config{
		User:     user,
		Password: password,
		Host:     container.Host,
		Port:     uint16(container.DefaultPort()),
		DBName:   database,
	},
		WithDefaultPoolConfig(),
		WithHealthCheckPeriod(10*time.Millisecond),
	)
	require.NoError(t, err)
	defer pgpool.Close()

	healthController := healing.New()
	healthController.AddReadyChecker("postgresql_controller", pgpool.CheckReadinessProber)

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go Run(workloadCtx, t, pgpool)
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
				// TODO: maybe need write more stable workload, but some times
				// check passed with ok.
				if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusOK {
					t.Fatalf("unexpected status %s", resp.Status)
				}
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Create a temp table using single row from target table. Later,
	// transaction will be rolled back and temp table will be dropped. Also, any errors could
	// be ignored, because in this case transaction (aborted) also stay idle.
	err = createTempTable(tx)
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
func createTempTable(tx pgx.Tx) error {
	temp := time.Now().Unix()
	q := fmt.Sprintf("CREATE TEMP TABLE temp_%d (c INT); SELECT * FROM temp_%d LIMIT 1", temp, temp)
	_, err := tx.Exec(context.Background(), q)
	if err != nil {
		return err
	}

	return nil
}
