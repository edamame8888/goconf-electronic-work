// 液晶を単色で順番に塗りつぶし、表示と配線を確かめる。
// 進み具合は USB シリアルへ出力する。
package main

import (
	"time"

	"github.com/edamame8888/goconf-electronic-work/internal/badge"
)

const (
	// startupDelay は USB シリアルの接続と液晶の電源の安定を待つ時間。
	startupDelay = time.Second
	// fillInterval は塗りつぶす色を切り替える間隔。
	fillInterval = time.Second
)

// fillColors は塗りつぶす色 (RGB565)。
var fillColors = []struct {
	name   string
	rgb565 uint16
}{
	{"red", 0xF800},
	{"green", 0x07E0},
	{"blue", 0x001F},
	{"white", 0xFFFF},
	{"black", 0x0000},
}

func main() {
	time.Sleep(startupDelay)
	println("lcdtest: start")

	display, err := badge.NewDisplay()
	for err != nil {
		println("lcdtest: init error:", err.Error())
		time.Sleep(fillInterval)
	}
	println("lcdtest: init done")

	for {
		for _, c := range fillColors {
			println("lcdtest: fill", c.name)
			err := display.FillRGB565(c.rgb565)
			if err != nil {
				println("lcdtest: fill error:", err.Error())
			}
			time.Sleep(fillInterval)
		}
	}
}
