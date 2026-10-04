// 液晶につながるピンを一定時間ごとに全て HIGH / LOW に切り替える。
// テスターで液晶側のピンの電圧を測り、信号が届いているかを確かめるために使う。
package main

import (
	"machine"
	"time"

	"github.com/edamame8888/goconf-electronic-work/internal/badge"
)

// holdTime は HIGH / LOW を保持する時間。テスターの表示が安定するよう長めにとる。
const holdTime = 5 * time.Second

var pins = []machine.Pin{
	badge.DisplaySCK,
	badge.DisplaySDO,
	badge.DisplayBL,
	badge.DisplayCS,
	badge.DisplayDC,
	badge.DisplayRST,
}

func main() {
	for _, p := range pins {
		p.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

	for {
		for _, p := range pins {
			p.High()
		}
		println("levelcheck: all HIGH (3.3V)")
		time.Sleep(holdTime)

		for _, p := range pins {
			p.Low()
		}
		println("levelcheck: all LOW (0V)")
		time.Sleep(holdTime)
	}
}
