package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/w0rxbend/instachron/services/ffmpeg-streamer/internal/ipcclient"
)

func run(ctx context.Context, cfg config, ipc *ipcclient.Reader, logger *log.Logger) error {
	if cfg.mergeAll {
		logger.Printf("merging all cameras via IPC %s at %d fps", cfg.socketPath, cfg.frameRate)
	} else {
		logger.Printf("watching camera=%d via IPC %s at %d fps", cfg.cameraID, cfg.socketPath, cfg.frameRate)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := runFFmpegSession(ctx, cfg, ipc, logger)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			logger.Printf("ffmpeg session ended: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.restartDelay):
			logger.Printf("restarting ffmpeg")
		}
	}
}

func runFFmpegSession(ctx context.Context, cfg config, ipc *ipcclient.Reader, logger *log.Logger) error {
	args := ffmpegArgs(cfg)
	cmd := exec.CommandContext(ctx, cfg.ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open ffmpeg stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	var feedErr error
	if cfg.mergeAll {
		feedErr = feedMergedFrames(ctx, cfg, ipc, stdin, logger)
	} else {
		feedErr = feedFrames(ctx, cfg, ipc, stdin)
	}
	_ = stdin.Close()

	waitErr := cmd.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	if feedErr != nil {
		return feedErr
	}
	if waitErr != nil {
		return fmt.Errorf("ffmpeg exited: %w", waitErr)
	}
	return nil
}

func ffmpegArgs(cfg config) []string {
	gop := cfg.frameRate * 2
	return []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-f", "mjpeg",
		"-framerate", strconv.Itoa(cfg.frameRate),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(cfg.frameRate),
		"-g", strconv.Itoa(gop),
		"-f", "flv",
		cfg.streamURL,
	}
}
