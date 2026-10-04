// 液晶につないだピンのショートを調べ、結果を USB シリアルへ出力する。
// 各ピンを内部プルアップ/プルダウンで読み、GND・3V3 とのショートや
// ピン同士のショートを検出する。
package main

import (
	"machine"
	"time"
)

type lcdPin struct {
	name string
	pin  machine.Pin
}

// lcdPins は液晶につないだピン。ピン配置は gocon2026badge 基板に合わせる。
var lcdPins = []lcdPin{
	{"CLK/SCL", machine.GPIO10},
	{"DIN/SDA", machine.GPIO11},
	{"BL", machine.GPIO12},
	{"CS", machine.GPIO13},
	{"DC", machine.GPIO14},
	{"RST", machine.GPIO15},
}

const (
	// settleTime はピン設定を変えてから読み取るまでの待ち時間。
	settleTime = 10 * time.Millisecond
	// reportInterval は結果を出力する間隔。
	reportInterval = 3 * time.Second
)

func main() {
	for {
		report()
		time.Sleep(reportInterval)
	}
}

// readWith はピンを指定の入力モードにして値を読む。
func readWith(p machine.Pin, mode machine.PinMode) bool {
	p.Configure(machine.PinConfig{Mode: mode})
	time.Sleep(settleTime)
	return p.Get()
}

func report() {
	println("=== LCD pin check ===")

	// GND・3V3 とのショートを調べる。
	stuckLow := make([]bool, len(lcdPins))
	for i, lp := range lcdPins {
		up := readWith(lp.pin, machine.PinInputPullup)
		down := readWith(lp.pin, machine.PinInputPulldown)
		stuckLow[i] = !up
		verdict := "OK (floating input)"
		switch {
		case !up && !down:
			verdict = "NG: stuck LOW (short to GND?)"
		case up && down:
			verdict = "stuck HIGH (short to 3V3 or pull-up on module)"
		case !up && down:
			verdict = "unstable"
		}
		println(lp.name, "GPIO", uint8(lp.pin), "pullup:", up, "pulldown:", down, "->", verdict)
	}

	// LOW に固定されていたピンは、HIGH を出力して読み返す。
	// 読み返しが LOW なら GND と直接ショートしている。HIGH なら液晶側のプルダウン等で異常ではない。
	for i, lp := range lcdPins {
		if !stuckLow[i] {
			continue
		}
		lp.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
		lp.pin.High()
		time.Sleep(settleTime)
		driven := lp.pin.Get()
		lp.pin.Low()
		lp.pin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		if driven {
			println(lp.name, "drive HIGH readback: true -> OK (pulled down on module, not a short)")
		} else {
			println(lp.name, "drive HIGH readback: false -> NG: hard short to GND")
		}
	}

	// ピン同士のショートを調べる。1 本だけ LOW を出力し、他のピンが引きずられるかを見る。
	shorts := 0
	for i, a := range lcdPins {
		a.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
		a.pin.Low()
		for j, b := range lcdPins {
			if j == i || stuckLow[j] {
				continue
			}
			if !readWith(b.pin, machine.PinInputPullup) {
				println("NG: short between", a.name, "and", b.name)
				shorts++
			}
		}
		a.pin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	if shorts == 0 {
		println("no short between signal pins")
	}
}
