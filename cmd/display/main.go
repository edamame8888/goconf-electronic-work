// RP2040-Zero に接続した ST7789 液晶 (240x240) を単色で順番に塗りつぶしつつ、
// 本体のオンボード RGB LED (WS2812) も並行して色を切り替える。
package main

import (
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/drivers/ws2812"
)

const (
	// ledBrightness はオンボード LED の明るさ (0-255)。最大だと眩しいため抑える。
	ledBrightness = 32
	// ledInterval はオンボード LED の色を切り替える間隔。
	ledInterval = 500 * time.Millisecond
	// displayInterval は液晶の色を切り替える間隔。
	displayInterval = time.Second
)

var ledColors = []color.RGBA{
	{R: 0xff, A: 0xff},                   // 赤
	{G: 0xff, A: 0xff},                   // 緑
	{B: 0xff, A: 0xff},                   // 青
	{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, // 白
}

var displayColors = []color.RGBA{
	{R: 255, G: 0, B: 0, A: 255},
	{R: 0, G: 255, B: 0, A: 255},
	{R: 0, G: 0, B: 255, A: 255},
	{R: 255, G: 255, B: 255, A: 255},
	{R: 0, G: 0, B: 0, A: 255},
}

func main() {
	// 液晶は故障の疑いがあるため一旦停止し、LED のみ動かす。
	// 液晶を再開するときは、以下のコメントアウトを戻して runLED を goroutine で動かす。
	// go runLED()
	//
	// time.Sleep(300 * time.Millisecond)
	// runDisplay()
	runLED()
}

// runLED はオンボード RGB LED の色を順番に切り替え続ける。
func runLED() {
	led := ws2812.NewWS2812(machine.WS2812)
	led.SetBrightness(ledBrightness)

	for i := 0; ; i = (i + 1) % len(ledColors) {
		led.WriteColors([]color.RGBA{ledColors[i]})
		time.Sleep(ledInterval)
	}
}

// runDisplay は液晶を単色で順番に塗りつぶし続ける。
func runDisplay() {
	// 液晶への送信にSPI1を使う。
	// 受信線（SDI/MISO）は使用しない。
	spi := machine.SPI1
	err := spi.Configure(machine.SPIConfig{
		Frequency: 8_000_000,
		Mode:      0,
		SCK:       machine.GPIO10,
		SDO:       machine.GPIO11,
		SDI:       machine.NoPin,
	})
	if err != nil {
		panic(err)
	}

	// 引数の順番：SPI、RST、DC、CS、BL
	// ピン配置は gocon2026badge 基板に合わせる。
	// https://github.com/sago35/keyboards#gocon2026badge
	display := st7789.New(
		spi,
		machine.GPIO15,
		machine.GPIO14,
		machine.GPIO13,
		machine.GPIO12,
	)

	display.Configure(st7789.Config{
		Width:    240,
		Height:   240,
		Rotation: st7789.NO_ROTATION,
	})

	display.InvertColors(true)
	display.EnableBacklight(true)

	for {
		for _, c := range displayColors {
			display.FillScreen(c)
			time.Sleep(displayInterval)
		}
	}
}
