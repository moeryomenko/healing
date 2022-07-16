package checkers

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/moeryomenko/healing"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_PGReadiness(t *testing.T) {
	// start postgresql.
	pg := postgres.Preset(
		postgres.WithUser(user, password),
		postgres.WithDatabase(database),
	)
	container, err := gnomock.Start(pg)
	require.NoError(t, err)
	defer func() { gnomock.Stop(container) }()

	// create our pg connections pool.
	poolConfig, err := pgxpool.ParseConfig(
		fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s pool_max_conns=2",
			user, password, container.Host, container.DefaultPort(), database))
	require.NoError(t, err)

	pgpool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	defer pgpool.Close()

	healthController := healing.New(8080)
	healthController.AddReadyChecker("postgresql_controller", PgxReadinessProber(pgpool))

	// run workload.
	workloadCtx, workloadCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer workloadCancel()
	go RunPgLoad(workloadCtx, t, pgpool)
	// run readiness controller.
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer healthCancel()
	go healthController.Heartbeat(healthCtx)
	defer func() {
		healthController.Stop(context.Background())
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

func RunPgLoad(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			go func() {
				startPgIdleXact(ctx, pool)
			}()
		}
	}
}

// startSingleIdleXact starts transaction and goes sleeping for specified amount of time.
func startPgIdleXact(ctx context.Context, pool *pgxpool.Pool) {
	_ = pool.BeginTxFunc(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Create a temp table using single row from target table. Later,
		// transaction will be rolled back and temp table will be dropped. Also, any errors could
		// be ignored, because in this case transaction (aborted) also stay idle.
		temp := time.Now().Unix()
		q := fmt.Sprintf("CREATE TEMPORARY TABLE temp_%d (c INT)", temp)
		_, err := tx.Exec(ctx, q)
		if err != nil {
			return err
		}
		defer func() {
			tx.Rollback(context.Background())
		}()

		// Stop execution only if context has been done or naptime interval is timed out.
		<-ctx.Done()
		return nil
	})
}
