package contract

import (
	"context"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Updater finds Team Follow's newest stable release on GitHub (ADR 0009).
// It talks only to GitHub's own hosts, over HTTPS, and sends nothing about
// the player. Downloading, installing and restarting come next (issue 23)
// and join this interface as Download, Install and Restart.
type Updater interface {
	// Latest returns the newest stable release, never a pre-release such
	// as edge. A zero Release (empty Tag) means there is none. The release
	// carries CHANGELOG.md at its tag when GitHub had one.
	Latest(ctx context.Context) (dto.Release, error)
}

// RetryLater is implemented by an Updater error that knows when GitHub
// allows the next request, such as its rate limit, so the companion waits
// until then without parsing the message.
type RetryLater interface {
	RetryAt() time.Time
}
