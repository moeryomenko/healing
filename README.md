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
	"github.com/moeryomenko/healing/checkers"
	"github.com/moeryomenko/squad"
)

func main() {
	ctx := context.Background()

	// create health/readiness controller.
	h := healing.New(8081 // health controller port.
		healing.WithCheckPeriod(3 * time.Second),
		healing.WithReadinessTimeout(time.Second),
		healing.WithReadyEndpoint("/readz"),
	)

	// add pool readiness controller to readiness group.
	h.AddReadyChecker("pgx", checkers.PgxReadinessProber(pool))

	// create squad group runner.
	s := squad.NewSquad(squad.WithSiganlHandler())

	// run health/readiness controller in squad group.
	s.RunGracefully(h.Heartbeat, h.Stop)

	...

	s.Wait()
}
```

## License

Healing is primarily distributed under the terms of both the MIT license and Apache License (Version 2.0).

See [LICENSE-APACHE](LICENSE-APACHE) and/or [LICENSE-MIT](LICENSE-MIT) for details.
