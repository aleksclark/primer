// Command content-recover is a one-time operator tool that restages legacy
// completed YouTube MP4s (no [id] in names, no sidecars) into Primer's
// yt-dlp finalize layout. Dry-run is the default. --apply hardlinks into
// output staging and calls ytdlp.FinalizeStaging; it never copies or writes
// the source tree.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aleksclark/primer/server/internal/ingest/recovery"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("content-recover", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	sourceRoot := fs.String("source-root", "", "parent of slug directories (e.g. /mnt/moosefs/media/Shows)")
	outputDir := fs.String("output-dir", "", "Primer library root; finalize writes Shows/<slug>/...")
	manifestPath := fs.String("manifest", "", "path to curriculum/content-manifest.yaml")
	slugsCSV := fs.String("slugs", "", "comma-separated manifest slugs")
	apply := fs.Bool("apply", false, "hardlink + finalize (default: dry-run, no directories or writes)")
	var slugFlags stringList
	fs.Var(&slugFlags, "slug", "manifest slug (repeatable)")

	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `content-recover — restage legacy YouTube MP4s via existing FinalizeStaging

Dry-run by default (JSON plan only; no directories or writes).
Pass --apply to hardlink into output staging and batch-finalize per slug.

Source files are never copied or modified. Output must not overlap source.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	slugs := recovery.NormalizeSlugs(*slugsCSV, strings.Join(slugFlags, ","))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := recovery.Run(ctx, recovery.Options{
		SourceRoot:   *sourceRoot,
		OutputDir:    *outputDir,
		ManifestPath: *manifestPath,
		Slugs:        slugs,
		Apply:        *apply,
	})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}
