// gocon2026badge 基板の RGB LED (SK6812MINI-E x 16) に虹色を流す。
package main

import (
	"image/color"
	"time"

	"github.com/edamame8888/goconf-electronic-work/internal/badge"
)

const (
	// brightness は LED の明るさ (0-255)。16 個同時点灯で眩しく電流も増えるため抑える。
	brightness = 32
	// hueStep は 1 フレームごとに色相を進める量 (0-359)。
	hueStep = 4
	// frameInterval は表示を更新する間隔。
	frameInterval = 30 * time.Millisecond
)

func main() {
	leds := badge.NewLEDs()
	leds.SetBrightness(brightness)

	buf := make([]color.RGBA, badge.NumLEDs)
	for phase := 0; ; phase = (phase + hueStep) % 360 {
		for i := range buf {
			buf[i] = badge.HueToRGBA((phase + i*360/badge.NumLEDs) % 360)
		}
		leds.WriteColors(buf)
		time.Sleep(frameInterval)
	}
}
