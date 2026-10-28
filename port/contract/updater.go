package contract

import (
	"context"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Updater finds Team Follow's newest stable release on GitHub and installs
// it (ADR 0009). It talks only to GitHub's own hosts, over HTTPS, and sends
// nothing about the player. Starting the new version is the composition
// root's job once the window has closed (crashreport).
type Updater interface {
	// Latest returns the newest stable release, never a pre-release such
	// as edge. A zero Release (empty Tag) means there is none. The release
	// carries CHANGELOG.md at its tag when GitHub had one.
	Latest(ctx context.Context) (dto.Release, error)
	// Download fetches the release's file for this system into a temporary
	// folder beside the installed app, reporting progress, and checks its
	// SHA-256 against the release's checksums.txt. It returns the verified
	// file's path; on any error, a cancelled ctx included, nothing is kept.
	Download(ctx context.Context, rel dto.Release, progress func(dto.UpdateProgress)) (string, error)
	// Install unpacks a file Download verified and replaces the installed
	// app with it. On error the installed app is as it was.
	Install(ctx context.Context, path string) error
	// Cleanup removes what an earlier update left behind: the replaced
	// program and temporary folders. It runs at every start.
	Cleanup(ctx context.Context) error
}

// UpdateProblem is implemented by an Updater error the player can act on,
// such as a folder Team Follow can't write to. Problem is a full sentence
// for the window.
type UpdateProblem interface {
	Problem() string
}

// RetryLater is implemented by an Updater error that knows when GitHub
// allows the next request, such as its rate limit, so the companion waits
// until then without parsing the message.
type RetryLater interface {
	RetryAt() time.Time
}
