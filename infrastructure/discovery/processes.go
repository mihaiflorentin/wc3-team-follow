package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

const (
	// closeGrace is how long a game asked to close may take before it is
	// forced.
	closeGrace = 3 * time.Second
	closePoll  = 100 * time.Millisecond
)

// Processes implements contract.GameProcesses. It closes only processes
// discovery identifies as the Warcraft III game executable, and only this
// user's where the operating system says who owns a process.
type Processes struct {
	platform platform
	logger   *slog.Logger
	grace    time.Duration
	poll     time.Duration
}

// NewProcesses returns the game process closer for this operating system.
func NewProcesses(logger *slog.Logger) contract.GameProcesses {
	return newProcesses(newPlatform(), logger)
}

func newProcesses(p platform, logger *slog.Logger) *Processes {
	return &Processes{platform: p, logger: logger, grace: closeGrace, poll: closePoll}
}

// CloseAll asks every game process of this user to close, waits up to the
// grace period, then forces those still running. It returns how many
// processes it asked to close.
func (p *Processes) CloseAll(ctx context.Context) (int, error) {
	all, err := p.platform.findGames(ctx)
	if err != nil {
		return 0, fmt.Errorf("list game processes: %w", err)
	}
	games := p.own(all)
	if skipped := len(all) - len(games); skipped > 0 {
		p.logger.InfoContext(ctx, "discovery.close_skipped", slog.Int("count", skipped))
	}
	var asked []int
	var errs []error
	for _, g := range games {
		if err := p.platform.stop(g, false); err != nil {
			errs = append(errs, fmt.Errorf("close game process %d: %w", g.pid, err))
			continue
		}
		asked = append(asked, g.pid)
	}
	left, err := p.waitGone(ctx, asked)
	if err != nil {
		errs = append(errs, err)
	}
	var forced []int
	for _, g := range left {
		if err := p.platform.stop(g, true); err != nil {
			errs = append(errs, fmt.Errorf("force game process %d to close: %w", g.pid, err))
			continue
		}
		forced = append(forced, g.pid)
	}
	p.logger.InfoContext(ctx, "discovery.close", slog.Any("pids", asked), slog.Any("forced", forced), slog.Int("failed", len(errs)))
	return len(asked), errors.Join(errs...)
}

// own keeps the games this user may close.
func (p *Processes) own(games []game) []game {
	var out []game
	for _, g := range games {
		if g.pid > 0 && p.platform.closable(g) {
			out = append(out, g)
		}
	}
	return out
}

// waitGone waits until none of pids is a running game, or the grace
// period ends, and returns those still running. Each check lists the game
// processes again, so a pid the system reused for another program is never
// forced.
func (p *Processes) waitGone(ctx context.Context, pids []int) ([]game, error) {
	if len(pids) == 0 {
		return nil, nil
	}
	deadline := time.Now().Add(p.grace)
	for {
		games, err := p.platform.findGames(ctx)
		if err != nil {
			return nil, fmt.Errorf("list game processes: %w", err)
		}
		var left []game
		for _, g := range p.own(games) {
			if slices.Contains(pids, g.pid) {
				left = append(left, g)
			}
		}
		if len(left) == 0 || !time.Now().Before(deadline) {
			return left, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.poll):
		}
	}
}
