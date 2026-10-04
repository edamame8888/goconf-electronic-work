package badge

import (
	"errors"
	"machine"
	"time"
)

const (
	// BytesPerPixel は RGB565 の 1 画素のバイト数。
	BytesPerPixel = 2

	// displaySPIFrequency は液晶との SPI 通信速度。ジャンパー線での接続でも安定する範囲にとどめる。
	displaySPIFrequency = 8_000_000
	// displayResetDelay はハードウェアリセットの各段階で待つ時間。
	displayResetDelay = 100 * time.Millisecond
	// displaySleepOutDelay は SLPOUT 後に待つ時間 (データシートでは 120ms 以上)。
	displaySleepOutDelay = 120 * time.Millisecond
)

// ST7789 のコマンド。
const (
	cmdSLPOUT  = 0x11
	cmdINVON   = 0x21
	cmdDISPON  = 0x29
	cmdCASET   = 0x2A
	cmdRASET   = 0x2B
	cmdRAMWR   = 0x2C
	cmdMADCTL  = 0x36
	cmdCOLMOD  = 0x3A
	cmdPORCTRL = 0xB2
	cmdGCTRL   = 0xB7
	cmdVCOMS   = 0xBB
	cmdLCMCTRL = 0xC0
	cmdVDVVRH  = 0xC2
	cmdVRHS    = 0xC3
	cmdVDVS    = 0xC4
	cmdFRCTRL2 = 0xC6
	cmdPWCTRL1 = 0xD0
	cmdPVGAM   = 0xE0
	cmdNVGAM   = 0xE1
)

var (
	// ErrOutOfBounds は描画範囲が画面からはみ出していることを表す。
	ErrOutOfBounds = errors.New("badge: area is out of the display")
	// ErrPixelsSize は画素データの長さが描画範囲と合わないことを表す。
	ErrPixelsSize = errors.New("badge: pixel data size does not match the area")
)

// Display は液晶 (ST7789) を SPI で直接制御する。
// 初期化手順は Waveshare 1.54inch LCD Module のデモ (LCD_1IN54_InitReg) に合わせる。
type Display struct {
	spi *machine.SPI
	// row は 1 行分の転送用バッファ。
	row []byte
}

// NewDisplay は SPI1 と液晶を初期化し、バックライトを点けた Display を返す。
func NewDisplay() (Display, error) {
	// 液晶は送信のみのため、受信線（SDI/MISO）は使用しない。
	spi := machine.SPI1
	err := spi.Configure(machine.SPIConfig{
		Frequency: displaySPIFrequency,
		Mode:      0,
		SCK:       DisplaySCK,
		SDO:       DisplaySDO,
		SDI:       machine.NoPin,
	})
	if err != nil {
		return Display{}, err
	}

	for _, p := range []machine.Pin{DisplayDC, DisplayCS, DisplayRST, DisplayBL} {
		p.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	DisplayCS.High()

	d := Display{spi: spi, row: make([]byte, DisplayWidth*BytesPerPixel)}
	d.reset()
	err = d.initReg()
	if err != nil {
		return Display{}, err
	}
	DisplayBL.High()
	return d, nil
}

// FillRGB565 は画面全体を 1 色 (RGB565) で塗りつぶす。
func (d Display) FillRGB565(c uint16) error {
	for i := 0; i < len(d.row); i += BytesPerPixel {
		d.row[i] = byte(c >> 8)
		d.row[i+1] = byte(c)
	}
	return d.writeRows(0, 0, DisplayWidth, DisplayHeight, func([]byte, int) {})
}

// DrawRGB565 は (x, y) を左上とする w×h の範囲に画素データを描く。
// pixels は RGB565 (ビッグエンディアン) の画素を行順に並べたもの。
// フラッシュに置いた埋め込みデータを RAM へ複製せずに渡せるよう string で受け取る。
func (d Display) DrawRGB565(x, y, w, h int, pixels string) error {
	if len(pixels) != w*h*BytesPerPixel {
		return ErrPixelsSize
	}
	rowBytes := w * BytesPerPixel
	return d.writeRows(x, y, w, h, func(row []byte, i int) {
		copy(row, pixels[i*rowBytes:(i+1)*rowBytes])
	})
}

// writeRows は描画範囲を設定し、fillRow で 1 行ずつ埋めたデータを送る。
func (d Display) writeRows(x, y, w, h int, fillRow func(row []byte, i int)) error {
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > DisplayWidth || y+h > DisplayHeight {
		return ErrOutOfBounds
	}
	x1, y1 := x+w-1, y+h-1
	err := d.command(cmdCASET, byte(x>>8), byte(x), byte(x1>>8), byte(x1))
	if err != nil {
		return err
	}
	err = d.command(cmdRASET, byte(y>>8), byte(y), byte(y1>>8), byte(y1))
	if err != nil {
		return err
	}

	row := d.row[:w*BytesPerPixel]
	DisplayCS.Low()
	defer DisplayCS.High()
	DisplayDC.Low()
	err = d.spi.Tx([]byte{cmdRAMWR}, nil)
	if err != nil {
		return err
	}
	DisplayDC.High()
	for i := 0; i < h; i++ {
		fillRow(row, i)
		err = d.spi.Tx(row, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// reset はハードウェアリセットを行う。
func (d Display) reset() {
	DisplayRST.High()
	time.Sleep(displayResetDelay)
	DisplayRST.Low()
	time.Sleep(displayResetDelay)
	DisplayRST.High()
	time.Sleep(displayResetDelay)
}

// command はコマンドと続くデータを、CS を 1 回下げた間に送る。
func (d Display) command(cmd byte, data ...byte) error {
	DisplayCS.Low()
	defer DisplayCS.High()
	DisplayDC.Low()
	err := d.spi.Tx([]byte{cmd}, nil)
	if err != nil || len(data) == 0 {
		return err
	}
	DisplayDC.High()
	return d.spi.Tx(data, nil)
}

// initReg は Waveshare のデモと同じ手順でレジスタを初期化する。
func (d Display) initReg() error {
	steps := []struct {
		cmd  byte
		data []byte
	}{
		{cmdMADCTL, []byte{0x00}},
		{cmdCOLMOD, []byte{0x05}}, // RGB565
		{cmdPORCTRL, []byte{0x0C, 0x0C, 0x00, 0x33, 0x33}},
		{cmdGCTRL, []byte{0x35}},
		{cmdVCOMS, []byte{0x19}},
		{cmdLCMCTRL, []byte{0x2C}},
		{cmdVDVVRH, []byte{0x01}},
		{cmdVRHS, []byte{0x12}},
		{cmdVDVS, []byte{0x20}},
		{cmdFRCTRL2, []byte{0x0F}},
		{cmdPWCTRL1, []byte{0xA4, 0xA1}},
		{cmdPVGAM, []byte{0xD0, 0x04, 0x0D, 0x11, 0x13, 0x2B, 0x3F, 0x54, 0x4C, 0x18, 0x0D, 0x0B, 0x1F, 0x23}},
		{cmdNVGAM, []byte{0xD0, 0x04, 0x0C, 0x11, 0x13, 0x2C, 0x3F, 0x44, 0x51, 0x2F, 0x1F, 0x1F, 0x20, 0x23}},
		{cmdINVON, nil},
		{cmdSLPOUT, nil},
	}
	for _, s := range steps {
		err := d.command(s.cmd, s.data...)
		if err != nil {
			return err
		}
	}
	time.Sleep(displaySleepOutDelay)
	return d.command(cmdDISPON)
}
