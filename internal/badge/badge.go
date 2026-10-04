// Package badge は gocon2026badge 基板のハードウェア設定をまとめる。
// ピン配置は https://github.com/sago35/keyboards#gocon2026badge に合わせる。
package badge

import (
	"image/color"
	"machine"

	"tinygo.org/x/drivers/ws2812"
)

const (
	// LEDPin は基板の LED 列 (SK6812MINI-E) につながるピン。
	LEDPin = machine.GPIO9
	// NumLEDs は基板に実装された LED の数。
	NumLEDs = 16

	// DisplayWidth と DisplayHeight は液晶 (ST7789) の解像度。
	DisplayWidth  = 240
	DisplayHeight = 240

	// 液晶 (ST7789) につながるピン。
	DisplaySCK = machine.GPIO10
	DisplaySDO = machine.GPIO11
	DisplayBL  = machine.GPIO12
	DisplayCS  = machine.GPIO13
	DisplayDC  = machine.GPIO14
	DisplayRST = machine.GPIO15
)

// NewLEDs は基板の LED 列のドライバを返す。
func NewLEDs() ws2812.Device {
	return ws2812.NewWS2812(LEDPin)
}

// HueToRGBA は色相 (0-359) を彩度・明度最大の色に変換する。
func HueToRGBA(hue int) color.RGBA {
	const segment = 60
	rise := uint8(hue % segment * 255 / segment)
	fall := 255 - rise
	switch hue / segment {
	case 0:
		return color.RGBA{R: 255, G: rise, A: 255}
	case 1:
		return color.RGBA{R: fall, G: 255, A: 255}
	case 2:
		return color.RGBA{G: 255, B: rise, A: 255}
	case 3:
		return color.RGBA{G: fall, B: 255, A: 255}
	case 4:
		return color.RGBA{R: rise, B: 255, A: 255}
	default:
		return color.RGBA{R: 255, B: fall, A: 255}
	}
}
