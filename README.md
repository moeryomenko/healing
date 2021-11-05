# Healing [![Go Reference](https://pkg.go.dev/badge/github.com/moeryomenko/healing.svg)](https://pkg.go.dev/github.com/moeryomenko/healing)

Healing is package contains a liveness and readiness controllers, compatible with
[squad](http://github.com/moeryomenko/squad) package. Also contains postgresql and mysql pools with readiness checker.

## Usage

```go
package main

import (
	"context"
	"time"

	"github.com/moeryomenko/healing"
	"github.com/moeryomenko/healing/decorators/pgx"
	"github.com/moeryomenko/squad"
)

func main() {
	ctx := context.Background()

	// create health/readiness controller.
	h := healing.New(
		healing.WithCheckPeriod(3 * time.Second),
		healing.WithReadinessTimeout(time.Second),
		healing.WithReadyEndpoint("/readz"),
	)

	// create postgresql pool.
	pool, err := pgx.New(ctx, pgx.Config{
		Host:     pgHost,
		Port:     pgPort,
		User:     pgUser,
		Password: pgPassword,
		DBName:   pgName,
	}, pgx.WithHealthCheckPeriod(100 * time.Millisecond)) // sets the duration between checks of the health of idle conn.
	if err != nil {
		// error handling.
		// ...
	}

	// add pool readiness controller to readiness group.
	h.AddReadyChecker(pool.CheckReadinessProber)

	// create squad group runner.
	s := squad.NewSquad(ctx, squad.WithSiganlHandler())

	// run health/readiness controller in squad group.
	s.Run(h.Heartbeat)

	...

	s.Wait()
}
```

## License

Healing is primarily distributed under the terms of both the MIT license and Apache License (Version 2.0).

See [LICENSE-APACHE](LICENSE-APACHE) and/or [LICENSE-MIT](LICENSE-MIT) for details.
