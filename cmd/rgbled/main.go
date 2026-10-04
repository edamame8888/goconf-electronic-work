// RP2040-Zero のオンボード RGB LED (WS2812) の色を順番に切り替える。
package main

import (
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers/ws2812"
)

const (
	// brightness は LED の明るさ (0-255)。オンボード LED は最大だと眩しいため抑える。
	brightness = 32
	// interval は色を切り替える間隔。
	interval = 500 * time.Millisecond
)

var colors = []color.RGBA{
	{R: 0xff, A: 0xff},                   // 赤
	{G: 0xff, A: 0xff},                   // 緑
	{B: 0xff, A: 0xff},                   // 青
	{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, // 白
}

func main() {
	led := ws2812.NewWS2812(machine.WS2812)
	led.SetBrightness(brightness)

	for i := 0; ; i = (i + 1) % len(colors) {
		led.WriteColors([]color.RGBA{colors[i]})
		time.Sleep(interval)
	}
}
