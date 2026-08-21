// detect-image runs YOLOv8 detection on a single JPEG file and writes an
// annotated copy, printing every detection to stdout.
//
// Usage:
//
//	detect-image <config.json> <input.jpg> [output.jpg]
//
// If output.jpg is omitted the result is written to "detected.jpg".
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/w0rxbend/instachron/services/camera-web-restream-detector-api/internal/config"
	"github.com/w0rxbend/instachron/services/camera-web-restream-detector-api/internal/detect"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: detect-image <config.json> <input.jpg> [output.jpg]")
		os.Exit(1)
	}

	configPath := os.Args[1]
	inputPath := os.Args[2]
	outputPath := "detected.jpg"
	if len(os.Args) > 3 {
		outputPath = os.Args[3]
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)

	// Unlike the long-running service, this one-shot tool was handed a config
	// path explicitly, so a config it cannot read is a mistake worth stopping
	// for rather than something to paper over with defaults.
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.Debug = true // always verbose for the one-shot tool

	det, err := detect.New(cfg, logger)
	if err != nil {
		log.Fatalf("init detector: %v", err)
	}
	defer det.Destroy()

	jpeg, err := os.ReadFile(inputPath)
	if err != nil {
		log.Fatalf("read %s: %v", inputPath, err)
	}

	dets, annotated, err := det.Detect(jpeg)
	if err != nil {
		log.Fatalf("detect: %v", err)
	}

	fmt.Printf("layout: %+v\n", det.Layout())
	fmt.Printf("detections: %d\n", len(dets))
	for i, d := range dets {
		fmt.Printf("  %d. [%s] conf=%.3f  box=(%.0f,%.0f)-(%.0f,%.0f)\n",
			i+1, d.ClassName, d.Confidence, d.X1, d.Y1, d.X2, d.Y2)
	}

	if err := os.WriteFile(outputPath, annotated, 0o644); err != nil {
		log.Fatalf("write %s: %v", outputPath, err)
	}
	fmt.Printf("annotated image saved to %s\n", outputPath)
}
