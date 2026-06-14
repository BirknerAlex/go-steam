package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	steam "github.com/BirknerAlex/go-steam"
)

func main() {
	var (
		itemID      = flag.Uint64("item", 0, "Steam Workshop item ID (PublishedFileID)")
		username    = flag.String("username", "", "Steam account username (required)")
		password    = flag.String("password", "", "Steam account password")
		totpSecret  = flag.String("totp-secret", "", "Base64 TOTP shared secret for auto Steam Guard (from mobile authenticator)")
		output      = flag.String("output", "./output", "Directory to write content into")
		cachePath   = flag.String("cache", "", "Cache directory for sessions/tokens/keys (default: ~/.cache/go-steam)")
		verbose     = flag.Bool("v", false, "Verbose logging")
	)
	flag.Parse()

	if *itemID == 0 {
		fmt.Fprintf(os.Stderr, "Error: -item is required\n")
		flag.Usage()
		os.Exit(1)
	}
	if *username == "" {
		fmt.Fprintf(os.Stderr, "Error: -username is required (workshop downloads require authentication)\n")
		flag.Usage()
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var guardCB steam.SteamGuardCallback
	switch {
	case *totpSecret != "":
		guardCB = steam.SteamGuardCodeGenerate(*totpSecret)
	default:
		guardCB = steam.InteractiveSteamGuard()
	}

	cfg := steam.Config{
		Username:           *username,
		Password:           *password,
		SteamGuardCallback: guardCB,
		CachePath:          *cachePath,
		Log:                log,
	}

	fmt.Fprintf(os.Stderr, "Connecting to Steam CM servers...\n")
	start := time.Now()

	client, err := steam.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Fprintf(os.Stderr, "Connected in %s\n", time.Since(start).Round(time.Millisecond))

	req := steam.WorkshopDownloadRequest{
		ItemID:    *itemID,
		TargetDir: *output,
	}

	fmt.Fprintf(os.Stderr, "Starting download: item=%d → %s\n", *itemID, *output)

	progressCh, err := client.DownloadWorkshopItem(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var lastPhase steam.Phase = -1
	var lastPct int
	downloadStart := time.Now()

	for p := range progressCh {
		if p.Err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", p.Err)
			os.Exit(1)
		}

		if p.Phase != lastPhase {
			fmt.Fprintf(os.Stderr, "\n[%s] %s...\n", time.Since(downloadStart).Round(time.Millisecond), p.Phase)
			lastPhase = p.Phase
		}

		if p.Phase == steam.PhaseDownloading && p.TotalBytes > 0 {
			pct := int(p.DoneBytes * 100 / p.TotalBytes)
			if pct != lastPct || p.TotalChunks > 0 && p.DoneChunks%100 == 0 {
				elapsed := time.Since(downloadStart)
				rate := ""
				if elapsed > 0 && p.DoneBytes > 0 {
					mbps := float64(p.DoneBytes) / elapsed.Seconds() / 1024 / 1024
					rate = fmt.Sprintf("  %.1f MB/s", mbps)
				}
				fmt.Fprintf(os.Stderr, "\r  %d%% [%d/%d chunks] %s%s   ",
					pct, p.DoneChunks, p.TotalChunks, formatBytes(p.DoneBytes), rate)
				lastPct = pct
			}
		}

		if p.Phase == steam.PhaseComplete {
			fmt.Fprintf(os.Stderr, "\n\nDone in %s\n", time.Since(downloadStart).Round(time.Millisecond))
		}
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
