package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

func newReplayCmd(c *container.ServiceContainer) *cobra.Command {
	return replayCmd(func() (replayDeps, error) {
		reader, err := c.ReplayReader()
		if err != nil {
			return replayDeps{}, err
		}
		objects, err := c.MapObjects()
		if err != nil {
			return replayDeps{}, err
		}
		locator, err := c.MapsLocator()
		if err != nil {
			return replayDeps{}, err
		}
		return replayDeps{Reader: reader, Objects: objects, Locator: locator}, nil
	})
}

// replayDeps is what the replay command needs.
type replayDeps struct {
	Reader contract.ReplayReader
	// Objects and Locator name the orders from the replay's map file.
	Objects contract.MapObjects
	Locator contract.MapsLocator
}

// replayCmd prints what a replay file holds, to check the replay parser.
// The text form leaves chat lines out (only their count); --json prints
// everything parsed, chat included. Orders are named from the replay's map
// when it is found in a Maps folder or given with --map.
func replayCmd(resolve func() (replayDeps, error)) *cobra.Command {
	var asJSON bool
	var mapFile string
	cmd := &cobra.Command{
		Use:   "replay <file.w3g>",
		Short: "Prints what Team Follow can read from a replay file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := resolve()
			if err != nil {
				return fmt.Errorf("replay: %w", err)
			}
			r, err := deps.Reader.ReadFile(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("replay: %w", err)
			}
			mapNote := nameOrders(cmd.Context(), deps, &r, mapFile)
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}
			return printReplay(out, r, mapNote)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print everything parsed as JSON, chat lines included")
	cmd.Flags().StringVar(&mapFile, "map", "", "the map file to name orders from (found in the Maps folders when left out)")
	return cmd
}

// nameOrders fills in the orders' names from the replay's map and returns a
// line saying where they came from, or why they couldn't be named.
func nameOrders(ctx context.Context, deps replayDeps, r *dto.Replay, mapFile string) string {
	if len(r.Orders) == 0 {
		return ""
	}
	if mapFile == "" {
		mapFile = findMap(ctx, deps.Locator, r.Map.Path)
	}
	if mapFile == "" {
		return "not named: " + r.Map.File + " isn't in a Maps folder; give it with --map"
	}
	names, err := deps.Objects.Names(ctx, mapFile)
	if err != nil {
		return "not named: " + err.Error()
	}
	for i := range r.Orders {
		r.Orders[i].Name = names[r.Orders[i].Code]
	}
	return "named from " + mapFile
}

// findMap looks for the replay's map, whose path is relative to the game
// folder ("Maps/Download/X.w3x"), in each Maps folder.
func findMap(ctx context.Context, locator contract.MapsLocator, gamePath string) string {
	rel := strings.ReplaceAll(gamePath, `\`, "/")
	if len(rel) < 5 || !strings.EqualFold(rel[:5], "Maps/") {
		return ""
	}
	for _, folder := range locator.MapsFolders(ctx) {
		path := filepath.Join(folder.Path, filepath.FromSlash(rel[5:]))
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

func printReplay(w io.Writer, r dto.Replay, mapNote string) error {
	names := map[int]string{}
	for _, p := range r.Players {
		names[p.ID] = playerLabel(p)
	}
	name := func(id int) string {
		if n, ok := names[id]; ok {
			return n
		}
		if id < 0 {
			return "unknown"
		}
		return fmt.Sprintf("player %d", id)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Game:      %s (%d), build %d", r.Version, r.VersionNumber, r.Build)
	if r.Post202 {
		b.WriteString(", 2.0.2+ format")
	}
	fmt.Fprintf(&b, "\nLength:    %s (time slots add up to %s)\n", r.Duration.Round(time.Second), r.GameTime.Round(time.Second))
	if r.Ended > 0 {
		fmt.Fprintf(&b, "Ended:     %s (the game clock stopped)\n", r.Ended.Round(10*time.Millisecond))
	}
	fmt.Fprintf(&b, "Game name: %s\nHost:      %s\nSaver:     %s\n", r.GameName, name(r.HostID), name(r.SaverID))
	fmt.Fprintf(&b, "Map:       %s by %s\n           %s\n           checksum %s", r.Map.File, r.Map.Creator, r.Map.Path, r.Map.Checksum)
	if r.Map.SHA1 != "" {
		fmt.Fprintf(&b, ", sha1 %s", r.Map.SHA1)
	}
	s := r.Settings
	fmt.Fprintf(&b, "\nSettings:  speed %d, observers %s, fixed teams %v, random races %v, random hero %v\n", s.Speed, s.Observers, s.FixedTeams, s.RandomRaces, s.RandomHero)

	b.WriteString("Players:\n")
	tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  slot\tid\tteam\tcolor\trace\thandicap\tplayer\tactions\tAPM\tleft")
	for _, p := range r.Players {
		kind := ""
		switch {
		case p.Observer:
			kind = " (observer)"
		case p.Computer:
			kind = fmt.Sprintf(" (AI %d)", p.AIStrength)
		}
		left := "-"
		if p.Left > 0 {
			left = p.Result + " at " + p.Left.Round(time.Second).String()
			if p.Result == "" {
				left = "at " + p.Left.Round(time.Second).String()
			}
		}
		_, _ = fmt.Fprintf(tw, "  %d\t%d\t%d\t%d\t%s\t%d%%\t%s%s\t%d (%d)\t%d APM\t%s\n",
			p.Slot, p.ID, p.Team, p.Color, p.Race, p.Handicap, playerLabel(p), kind, p.Actions, p.APMActions, p.APM, left)
	}
	_ = tw.Flush()

	fmt.Fprintf(&b, "Leaves:    %d\n", len(r.Leaves))
	for _, l := range r.Leaves {
		fmt.Fprintf(&b, "  %8s  %s: %s, %s (counter %d)\n", l.Time.Round(time.Second), name(l.PlayerID), l.Reason, l.Result, l.Counter)
	}
	modes := map[string]int{}
	for _, c := range r.Chat {
		modes[c.Mode]++
	}
	fmt.Fprintf(&b, "Chat:      %d lines", len(r.Chat))
	for _, m := range []string{"lobby", "all", "allies", "observers", "private", "unseen"} {
		if modes[m] > 0 {
			fmt.Fprintf(&b, ", %s %d", m, modes[m])
		}
	}
	b.WriteString(" (use --json to see them)\n")
	printOrders(&b, r, name, mapNote)
	switch {
	case r.Winner.Draw:
		fmt.Fprintf(&b, "Winner:    draw (%s: %s)\n", r.Winner.Confidence, r.Winner.Basis)
	case r.Winner.Team >= 0:
		fmt.Fprintf(&b, "Winner:    team %d (%s: %s)\n", r.Winner.Team, r.Winner.Confidence, r.Winner.Basis)
	default:
		fmt.Fprintf(&b, "Winner:    unknown (%s)\n", r.Winner.Basis)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(&b, "Warning:   %s\n", warn)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func playerLabel(p dto.ReplayPlayer) string {
	label := p.Name
	if p.BattleTag != "" && p.BattleTag != p.Name {
		label = p.BattleTag
	}
	if p.Clan != "" {
		label += " [" + p.Clan + "]"
	}
	return label
}

// printOrders prints each player's most frequent orders, named when the map
// was read.
func printOrders(b *strings.Builder, r dto.Replay, name func(int) string, mapNote string) {
	fmt.Fprintf(b, "Orders:    %d", len(r.Orders))
	if mapNote != "" {
		fmt.Fprintf(b, " (%s)", mapNote)
	}
	b.WriteString("\n")
	type count struct {
		label string
		n     int
	}
	byPlayer := map[int]map[string]int{}
	var ids []int
	for _, o := range r.Orders {
		if byPlayer[o.PlayerID] == nil {
			byPlayer[o.PlayerID] = map[string]int{}
			ids = append(ids, o.PlayerID)
		}
		label := o.Code
		if o.Name != "" {
			label = o.Name
		}
		byPlayer[o.PlayerID][label]++
	}
	sort.Ints(ids)
	for _, id := range ids {
		var counts []count
		for label, n := range byPlayer[id] {
			counts = append(counts, count{label, n})
		}
		sort.Slice(counts, func(i, j int) bool {
			if counts[i].n != counts[j].n {
				return counts[i].n > counts[j].n
			}
			return counts[i].label < counts[j].label
		})
		parts := make([]string, 0, 8)
		for _, c := range counts[:min(8, len(counts))] {
			parts = append(parts, fmt.Sprintf("%s ×%d", c.label, c.n))
		}
		fmt.Fprintf(b, "  %s: %s\n", name(id), strings.Join(parts, ", "))
	}
}
