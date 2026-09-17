package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/signal"
	"time"

	"github.com/kawapiki/Jonsbo-resurrection/internal/device"
	"github.com/kawapiki/Jonsbo-resurrection/internal/display"
	"github.com/kawapiki/Jonsbo-resurrection/internal/imageutil"
	trayui "github.com/kawapiki/Jonsbo-resurrection/internal/tray"
	"github.com/kawapiki/Jonsbo-resurrection/internal/winusb"
)

var version = "dev"
var revision = "unknown"
var guiBuild = "false"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "jonsbo:", err)
		if guiBuild == "true" && (len(os.Args) == 1 || os.Args[1] == "tray" || os.Args[1] == "startup") {
			trayui.ShowError(err)
		}
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return tray(nil)
	}
	switch args[0] {
	case "version", "--version":
		fmt.Printf("Jonsbo Resurrection %s (%s)\n", version, revision)
		return nil
	case "tray":
		return tray(args[1:])
	case "startup":
		return startup(args[1:])
	case "help", "--help", "-h":
		fmt.Println("Usage: jonsbo serve [--all] [--example] | stats | monitor --all | list | inspect --serial SERIAL | image --serial SERIAL --file IMAGE | pattern --serial SERIAL --number 1 [--preview FILE]")
		fmt.Println("Desktop: tray | startup enable [--elevated] | startup disable | startup status | stop --tray | version")
		return nil
	case "serve":
		return serve(args[1:])
	case "stop":
		name := monitorName
		if len(args) == 2 && args[1] == "--server" {
			name = serverName
		} else if len(args) == 2 && args[1] == "--tray" {
			name = trayName
		} else if len(args) != 1 {
			return fmt.Errorf("usage: stop [--server]")
		}
		return signalMonitorStop(name)
	case "stats":
		return stats(args[1:])
	case "monitor":
		return monitor(args[1:])
	case "list":
		ds, err := winusb.Enumerate()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ds)
	case "inspect":
		fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
		serial := fs.String("serial", "", "display serial from list")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		ds, err := winusb.Enumerate()
		if err != nil {
			return err
		}
		d, err := device.Select(ds, *serial)
		if err != nil {
			return err
		}
		c, err := winusb.Open(d)
		if err != nil {
			return err
		}
		defer c.Close()
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(c.Pipes)
	case "image", "pattern":
		return sendImage(args[0], args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func sendImage(command string, args []string) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	serial := fs.String("serial", "", "display serial from list")
	file := fs.String("file", "", "PNG or JPEG image file")
	number := fs.Int("number", 1, "pattern label, 1..4")
	rotate := fs.Int("rotate", -1, "clockwise rotation: 0, 90, 180, 270 (default: fans 90, pump 0)")
	preview := fs.String("preview", "", "save the rendered pattern/image as PNG")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if *number < 1 || *number > 4 {
		return fmt.Errorf("pattern number must be 1..4")
	}
	ds, err := winusb.Enumerate()
	if err != nil {
		return err
	}
	d, err := device.Select(ds, *serial)
	if err != nil {
		return err
	}
	w, h, err := display.Dimensions(d.Kind)
	if err != nil {
		return err
	}
	if *rotate == -1 {
		*rotate = 0
		if d.Kind == "fan" {
			*rotate = 90
		}
	}
	if *rotate != 0 && *rotate != 90 && *rotate != 180 && *rotate != 270 {
		return fmt.Errorf("rotation must be 0, 90, 180 or 270")
	}
	if *rotate == 90 || *rotate == 270 {
		w, h = h, w
	}
	var im image.Image
	if command == "pattern" {
		im = imageutil.Pattern(w, h, *number)
	} else {
		if *file == "" {
			return fmt.Errorf("image requires --file")
		}
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			f.Close()
			return err
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			f.Close()
			return fmt.Errorf("source image exceeds 40 megapixels")
		}
		if _, err = f.Seek(0, 0); err != nil {
			f.Close()
			return err
		}
		src, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		im, err = imageutil.Fit(src, w, h)
		if err != nil {
			return err
		}
	}
	if *preview != "" {
		f, err := os.Create(*preview)
		if err != nil {
			return err
		}
		err = png.Encode(f, im)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	im, err = imageutil.Rotate(im, *rotate)
	if err != nil {
		return err
	}
	c, err := winusb.Open(d)
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := display.Send(ctx, c, d.Kind, im)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if encodeErr := enc.Encode(struct {
		Serial string `json:"serial"`
		Kind   string `json:"kind"`
		display.Result
	}{d.Serial, d.Kind, result}); encodeErr != nil {
		return encodeErr
	}
	return err
}
