// gocon2026badge 基板で、液晶にゴーファー君のアニメーションを表示しつつ、
// LED 列に虹色を並行して流す。
package main

import (
	_ "embed"
	"image/color"
	"time"

	"github.com/edamame8888/goconf-electronic-work/internal/badge"
)

//go:generate go run ../../tools/genimage -dir . -rotate 90

// backgroundRGB565 は最初に 1 回だけ描く全画面の背景 (RGB565 ビッグエンディアン)。
// frameRGB565 は動く範囲 (anim_gen.go の spriteX など) だけを切り出した各コマを連結したもの。
// string にしておくと、RAM へ複製されずにフラッシュに置かれたまま使える。
var (
	//go:embed background.rgb565
	backgroundRGB565 string
	//go:embed frames.rgb565
	framesRGB565 string
)

const (
	// ledBrightness は LED の明るさ (0-255)。16 個同時点灯で眩しく電流も増えるため抑える。
	ledBrightness = 32
	// hueStep は 1 フレームごとに色相を進める量 (0-359)。
	hueStep = 4
	// ledFrameInterval は LED の表示を更新する間隔。
	ledFrameInterval = 30 * time.Millisecond

	// displayStartupDelay は液晶の電源が安定するまでの待ち時間。
	displayStartupDelay = 300 * time.Millisecond
	// errorRetryInterval は液晶でエラーが起きたときにエラーを出力し直す間隔。
	errorRetryInterval = time.Second
)

func main() {
	// 液晶の初期化より先に LED を動かし、プログラムが起動していることを確認できるようにする。
	go runLEDs()

	time.Sleep(displayStartupDelay)
	err := runAnimation()
	for {
		println("display error:", err.Error())
		time.Sleep(errorRetryInterval)
	}
}

// runLEDs は LED 列に虹色を流し続ける。
func runLEDs() {
	leds := badge.NewLEDs()
	leds.SetBrightness(ledBrightness)

	buf := make([]color.RGBA, badge.NumLEDs)
	for phase := 0; ; phase = (phase + hueStep) % 360 {
		for i := range buf {
			buf[i] = badge.HueToRGBA((phase + i*360/badge.NumLEDs) % 360)
		}
		leds.WriteColors(buf)
		time.Sleep(ledFrameInterval)
	}
}

// runAnimation は背景を描いた後、動く範囲だけをコマ送りで描き続ける。エラーのときだけ戻る。
func runAnimation() error {
	display, err := badge.NewDisplay()
	if err != nil {
		return err
	}
	err = display.DrawRGB565(0, 0, badge.DisplayWidth, badge.DisplayHeight, backgroundRGB565)
	if err != nil {
		return err
	}

	const frameBytes = spriteW * spriteH * badge.BytesPerPixel
	next := time.Now()
	for i := 0; ; i = (i + 1) % numFrames {
		start := time.Now()
		frame := framesRGB565[i*frameBytes : (i+1)*frameBytes]
		err := display.DrawRGB565(spriteX, spriteY, spriteW, spriteH, frame)
		if err != nil {
			return err
		}
		if i == 0 {
			println("frame draw time (ms):", time.Since(start).Milliseconds())
		}

		// 描画に時間がかかっても、コマの間隔が一定になるように待つ。
		next = next.Add(frameInterval)
		wait := time.Until(next)
		if wait > 0 {
			time.Sleep(wait)
		} else {
			next = time.Now()
		}
	}
}
