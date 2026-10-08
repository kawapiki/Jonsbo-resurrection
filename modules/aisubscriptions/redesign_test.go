package aisubscriptions

import (
	"bytes"
	"image"
	"image/color"
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

func TestMascotsRespondToActivityWithoutAccountConnection(t *testing.T) {
	now := time.Unix(1720000000, 0)
	for _, provider := range []string{data.OpenAI, data.Claude} {
		for _, activity := range []string{"running", "waiting"} {
			s := data.State{Sessions: []data.Session{{Provider: provider, Activity: activity}}}
			a := DrawWidget(s, provider, 640, 180, now, true).(*image.RGBA)
			b := DrawWidget(s, provider, 640, 180, now.Add(350*time.Millisecond), true).(*image.RGBA)
			if bytes.Equal(a.Pix, b.Pix) {
				t.Fatalf("%s %s activity did not animate", provider, activity)
			}
			still := DrawWidget(s, provider, 640, 180, now, false).(*image.RGBA)
			stillLater := DrawWidget(s, provider, 640, 180, now.Add(350*time.Millisecond), false).(*image.RGBA)
			if !bytes.Equal(still.Pix, stillLater.Pix) {
				t.Fatal("disabled activity animation moved")
			}
		}
	}
}

func TestStaleSessionsCannotAnimateDisconnectedMascots(t *testing.T) {
	now := time.Unix(1720000000, 0)
	for _, provider := range []string{data.OpenAI, data.Claude} {
		s := data.State{Sessions: []data.Session{{Provider: provider, Activity: "running", Stale: true}}}
		a := DrawWidget(s, provider, 640, 180, now, true).(*image.RGBA)
		b := DrawWidget(s, provider, 640, 180, now.Add(350*time.Millisecond), true).(*image.RGBA)
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Fatal("stale activity still animates")
		}
	}
}

func TestOpenAIBlossomHasWhiteStrokesWithoutAnIconBackground(t *testing.T) {
	now := time.Unix(1720000000, 0)
	dark := color.RGBA{0, 0, 0, 255}
	for _, activity := range []string{"", "running", "waiting"} {
		for _, animate := range []bool{false, true} {
			for _, offset := range []time.Duration{0, 350 * time.Millisecond, 2 * time.Second} {
				im := image.NewRGBA(image.Rect(0, 0, 128, 128))
				fill(im, im.Bounds(), dark)
				drawMascot(im, data.OpenAI, im.Bounds(), now.Add(offset), animate, activity)
				// The blossom's open centre must expose the display background.
				if im.RGBAAt(64, 64) != dark {
					t.Fatalf("%q animated=%t offset=%s: icon background fills the blossom centre", activity, animate, offset)
				}
				white := 0
				for y := 0; y < 128; y++ {
					for x := 0; x < 128; x++ {
						if im.RGBAAt(x, y) == (color.RGBA{255, 255, 255, 255}) {
							white++
						}
						if (x < 4 || x >= 124 || y < 4 || y >= 124) && im.RGBAAt(x, y) != dark {
							t.Fatalf("%q animated=%t offset=%s: icon background reaches the frame edge", activity, animate, offset)
						}
					}
				}
				if white < 200 || white > 128*128/2 {
					t.Fatalf("%q animated=%t offset=%s: expected isolated white strokes, got %d white pixels", activity, animate, offset, white)
				}
			}
		}
	}
}

func TestCompactTokenValuesKeepLargeNumbersReadable(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1200: "1.2K", 1240000: "1.2M", 2300000000: "2.3B"} {
		if got := compactNumber(&n); got != want {
			t.Errorf("%d: got %s, want %s", n, got, want)
		}
	}
	if compactNumber(nil) != "--" {
		t.Fatal("missing tokens became zero")
	}
}

func TestExpandedWidgetKeepsSessionContextVisibleAtMinimumHeight(t *testing.T) {
	used, limit := int64(50), int64(100)
	s := data.State{Sessions: []data.Session{{Provider: data.OpenAI, Title: "A visible session", Activity: "running", Context: &data.ContextUsage{Used: &used, Limit: &limit}}}}
	for _, height := range []int{300, 380, 480} {
		im := DrawWidgetDetail(s, data.OpenAI, "expanded", 608, height, time.Now(), false).(*image.RGBA)
		count := 0
		for y := 0; y < height; y++ {
			for x := 0; x < 608; x++ {
				if im.RGBAAt(x, y) == (color.RGBA{102, 170, 204, 255}) {
					count++
				}
			}
		}
		if count < 100 {
			t.Fatalf("%d px expanded widget clips session context", height)
		}
	}
}
