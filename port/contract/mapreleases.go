package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// MapReleases finds a tracked map's newest release on GitHub and downloads
// it into the Maps folder (ADR 0012). It talks only to GitHub's own hosts,
// over HTTPS, and sends nothing about the player. Errors the player can act
// on implement UpdateProblem; GitHub's rate limit implements RetryLater.
type MapReleases interface {
	// Latest returns the map's newest release, never a draft or a
	// pre-release. A zero MapRelease (empty Version) means there is none
	// with a file matching the map's pattern. A release without a SHA-256
	// digest is refused with an error.
	Latest(ctx context.Context, tracked dto.TrackedMap) (dto.MapRelease, error)
	// Download fetches the release's file into dir, reporting progress, and
	// checks it against the release's SHA-256 while it streams. Only a
	// matching file is moved into place, and never over an existing one;
	// it returns its path. On any error, a cancelled ctx included, nothing
	// is left in dir.
	Download(ctx context.Context, rel dto.MapRelease, dir string, progress func(dto.MapProgress)) (string, error)
}
