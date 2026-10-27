package main

import (
	"os"
	"testing"
	"time"
)

// timezone is the project's timezone: the Dockerfile sets it as TZ.
const timezone = "Europe/Bucharest"

// TestTimezoneResolves checks that the timezone (or $TZ, when set) loads,
// and that the process local time uses it. main.go imports time/tzdata, so
// this holds even without zoneinfo files on the host, as in a distroless or
// scratch image: run the compiled test binary there to check it, e.g.
// `GOOS=linux go test -c -o tz.test .` and run `./tz.test` with TZ set.
func TestTimezoneResolves(t *testing.T) {
	name := timezone
	if tz := os.Getenv("TZ"); tz != "" {
		name = tz
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("timezone %q does not resolve: %v", name, err)
	}
	if loc.String() != name {
		t.Fatalf("LoadLocation(%q) = %q", name, loc)
	}
	if os.Getenv("TZ") != "" && time.Local.String() != name {
		t.Fatalf("TZ=%s but the local timezone is %q: the timezone database is missing", name, time.Local)
	}
}
