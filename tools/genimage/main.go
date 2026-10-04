// genimage はバッジの液晶に表示するアニメーションを生成する。
// 正面向きのドット絵のゴーファー君が弾んだり手を振ったりし、
// 下に OPTiM のロゴとクレジットを表示する。
//
// 出力 (-dir で指定したディレクトリへ):
//   - background.rgb565: 最初に 1 回だけ描く全画面の背景
//   - frames.rgb565: 動く範囲だけを切り出した各コマを連結したもの
//   - anim_gen.go: 切り出し範囲・コマ数・コマ間隔の定数
//   - preview.gif: 確認用のアニメーション (回転前の向き)
//
// RGB565 はビッグエンディアンで、液晶の向きに合わせて -rotate 度 (時計回り) 回転させてある。
//
// The Go gopher was designed by Renée French. Illustrations by avocadoneko.
// https://go.dev/blog/gopher
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/format"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// logoPNG は下部に配置する OPTiM のロゴ。
//
//go:embed assets/optim_logo.png
var logoPNG []byte

const (
	width  = 240
	height = 240

	// supersample は文字とロゴを描くときの、1 画素あたりの縦横のサンプル数。輪郭をなめらかにする。
	supersample = 4

	// bgDotSpacing は背景に散らす点の間隔 (px)、bgDotSize は点の大きさ (px)。
	bgDotSpacing = 12
	bgDotSize    = 2

	// title は画面上部に表示する文字。titleSize は文字の大きさ (px)、titleBaseline はベースラインの y 座標。
	title         = "Go Conference 2026"
	titleSize     = 15.0
	titleBaseline = 20.0

	// logoWidth はロゴの幅 (px)、logoCenterY はロゴの中心の y 座標。
	logoWidth   = 104.0
	logoCenterY = 187.0

	// creditSize はクレジットの文字の大きさ (px)、creditBaseline は 1 行目のベースライン、creditLineHeight は行の間隔。
	creditSize       = 9.0
	creditBaseline   = 220.0
	creditLineHeight = 12.0

	// dotW, dotH はゴーファー君のドット絵の縦横のドット数、dotSize は 1 ドットの大きさ (px)。
	dotW    = 48
	dotH    = 46
	dotSize = 3
	// spriteLeft, spriteTop はドット絵の左上の位置 (px)。
	spriteLeft = (width - dotW*dotSize) / 2
	spriteTop  = 28
	// shadowRadiusX, shadowRadiusY は足元の影の半径 (px)。
	shadowRadiusX = 46.0
	shadowRadiusY = 4.0

	// numFrames はコマ数、frameDelayMs はコマの間隔 (ミリ秒)。
	numFrames    = 12
	frameDelayMs = 120

	// regionMargin は動く範囲の周りに余分に含める幅 (px)。
	regionMargin = 2
	// bytesPerPixel は RGB565 の 1 画素のバイト数。
	bytesPerPixel = 2
)

// creditLines は画面下部に表示するクレジット。
var creditLines = []string{
	"The Go gopher was designed by Renée French.",
	"Illustrations by avocadoneko.",
}

type rgb struct{ r, g, b float64 }

func hex(v uint32) rgb {
	return rgb{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}
}

func mix(a, b rgb, t float64) rgb {
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

var (
	bgColor      = hex(0xfff5c4)
	bgDotColor   = hex(0xf6e7a0)
	shadowColor  = hex(0xeedb8a)
	titleColor   = hex(0x0e2a47)
	creditColor  = hex(0x4a5566)
	outlineColor = hex(0x0e2a47)

	furLight   = hex(0x9ed6ec)
	furMid     = hex(0x6cbcdf)
	furDark    = hex(0x4f9fcb)
	furHilite  = hex(0xc4e9f6)
	eyeWhite   = hex(0xffffff)
	eyeShade   = hex(0xd5e4ee)
	pupilColor = hex(0x173a63)
	muzzle     = hex(0xe9cfb6)
	handLight  = hex(0xe8cfb5)
	handShade  = hex(0xcdb094)
	footLight  = hex(0xa3b1b8)
	footShade  = hex(0x7f8d94)
	toothColor = hex(0xffffff)
)

// ----- ドット絵のゴーファー君 -----

// pose はあるコマでのゴーファー君の姿勢。
type pose struct {
	bob   int  // 上に弾むドット数
	wave  int  // 0: 手を下ろす、1 と 2: 手を振る 2 つの姿勢
	blink bool // にっこり目を閉じる
}

// poseAt はコマ frame の姿勢を返す。2 回弾む間に手を振り、最後に瞬きする。
func poseAt(frame int) pose {
	bobs := [numFrames]int{0, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0}
	waves := [numFrames]int{0, 0, 0, 0, 1, 2, 1, 2, 0, 0, 0, 0}
	return pose{bob: bobs[frame], wave: waves[frame], blink: frame == 10}
}

// mask はドット絵の各ドットが図形に含まれるかを表す。
type mask [dotH][dotW]bool

// shapeMask は各ドットの中心が inside を満たすかでマスクを作る。
func shapeMask(inside func(x, y float64) bool) mask {
	var m mask
	for y := 0; y < dotH; y++ {
		for x := 0; x < dotW; x++ {
			m[y][x] = inside(float64(x)+0.5, float64(y)+0.5)
		}
	}
	return m
}

func (m *mask) at(x, y int) bool {
	return x >= 0 && y >= 0 && x < dotW && y < dotH && m[y][x]
}

// touches は (x, y) の上下左右のいずれかが o に含まれるかを返す。
func touches(o *mask, x, y int) bool {
	return o.at(x-1, y) || o.at(x+1, y) || o.at(x, y-1) || o.at(x, y+1)
}

// onEdge は (x, y) が m に含まれ、上下左右のいずれかが m の外かを返す。
func onEdge(m *mask, x, y int) bool {
	return m.at(x, y) && (!m.at(x-1, y) || !m.at(x+1, y) || !m.at(x, y-1) || !m.at(x, y+1))
}

func inEllipse(x, y, cx, cy, rx, ry, angle float64) bool {
	s, c := math.Sincos(angle)
	u, v := (x-cx)*c+(y-cy)*s, -(x-cx)*s+(y-cy)*c
	return (u/rx)*(u/rx)+(v/ry)*(v/ry) <= 1
}

// ドット絵の各部の位置 (ドット単位)。
const (
	bodyCX, bodyCY, bodyRX, bodyRY = 24.0, 23.0, 19.5, 18.5
	eyeCY, eyeR                    = 18.5, 8.0
	leftEyeCX, rightEyeCX          = 15.5, 32.5
)

func bodyMask() mask {
	return shapeMask(func(x, y float64) bool {
		// 角の丸い四角に近い丸 (超楕円)。頭の上に小さなくぼみを 2 つ入れる。
		const n = 2.6
		dx, dy := math.Abs(x-bodyCX)/bodyRX, math.Abs(y-bodyCY)/bodyRY
		if math.Pow(dx, n)+math.Pow(dy, n) > 1 {
			return false
		}
		for _, dip := range []float64{17.5, 30.5} {
			if math.Abs(x-dip) < 1.5 && y < bodyCY-bodyRY+1.5 {
				return false
			}
		}
		return true
	})
}

func handMask(p pose) mask {
	return shapeMask(func(x, y float64) bool {
		if inEllipse(x, y, 4.5, 29, 2.6, 4.6, 0.55) {
			return true
		}
		switch p.wave {
		case 1:
			return inEllipse(x, y, 44, 17.5, 2.6, 4.6, -2.4)
		case 2:
			return inEllipse(x, y, 45.2, 20, 2.6, 4.6, -2.0)
		default:
			return inEllipse(x, y, 43.5, 29, 2.6, 4.6, -0.55)
		}
	})
}

// furColor は体の毛の色。上から明るい・中間・濃いの 3 段で塗り、境目の 1 行は市松模様で混ぜる。右端は一段濃くする。
func furColor(x, y int) rgb {
	band := 0
	switch {
	case y > 30 || (y == 30 && (x+y)%2 == 0):
		band = 2
	case y > 13 || (y == 13 && (x+y)%2 == 0):
		band = 1
	}
	if float64(x) > bodyCX+bodyRX-4 && band < 2 {
		band++
	}
	return []rgb{furLight, furMid, furDark}[band]
}

// drawGopher はゴーファー君のドット絵を描く。返り値の set が false のドットは透明。
func drawGopher(p pose) (img [dotH][dotW]rgb, set mask) {
	body := bodyMask()
	ears := shapeMask(func(x, y float64) bool {
		return math.Hypot(x-9, y-8) < 4.2 || math.Hypot(x-39, y-8) < 4.2
	})
	hands := handMask(p)
	feet := shapeMask(func(x, y float64) bool {
		return y >= 40 && y < 44 && ((x >= 14 && x < 19) || (x >= 29 && x < 34))
	})
	eyes := shapeMask(func(x, y float64) bool {
		return math.Hypot(x-leftEyeCX, y-eyeCY) < eyeR || math.Hypot(x-rightEyeCX, y-eyeCY) < eyeR
	})
	muzzleM := shapeMask(func(x, y float64) bool {
		return inEllipse(x, y, 24, 27.5, 5.2, 3, 0)
	})

	put := func(x, y int, c rgb) {
		img[y][x] = c
		set[y][x] = true
	}
	for y := 0; y < dotH; y++ {
		for x := 0; x < dotW; x++ {
			switch {
			case hands.at(x, y):
				c := handLight
				if y > 29 {
					c = handShade
				}
				put(x, y, c)
			case feet.at(x, y):
				c := footLight
				if y >= 43 {
					c = footShade
				}
				if touches(&body, x, y) && !body.at(x, y) {
					c = outlineColor
				}
				put(x, y, c)
			case body.at(x, y):
				c := furColor(x, y)
				// 手・足との境目に線を入れる。
				if touches(&hands, x, y) || (touches(&feet, x, y) && y >= 38) {
					c = outlineColor
				}
				put(x, y, c)
			case ears.at(x, y):
				c := furMid
				if touches(&body, x, y) {
					c = outlineColor
				}
				put(x, y, c)
			}
		}
	}

	// 外側の輪郭線。
	silhouette := set
	for y := 0; y < dotH; y++ {
		for x := 0; x < dotW; x++ {
			if !silhouette.at(x, y) && touches(&silhouette, x, y) {
				put(x, y, outlineColor)
			}
		}
	}

	// 耳の内側の線と、頭の左上のつや。
	for _, d := range [][2]int{{8, 7}, {9, 8}, {39, 7}, {38, 8}} {
		put(d[0], d[1], outlineColor)
	}
	for x := 12; x <= 15; x++ {
		put(x, 7, furHilite)
	}

	// 口元・鼻・歯。
	for y := 0; y < dotH; y++ {
		for x := 0; x < dotW; x++ {
			if muzzleM.at(x, y) {
				put(x, y, muzzle)
			}
		}
	}
	for x := 22; x <= 26; x++ {
		put(x, 25, outlineColor)
	}
	for x := 23; x <= 25; x++ {
		put(x, 26, outlineColor)
	}
	for _, x := range []int{21, 22, 25, 26} {
		put(x, 31, toothColor)
		put(x, 32, toothColor)
	}

	// 目。
	for y := 0; y < dotH; y++ {
		for x := 0; x < dotW; x++ {
			if !eyes.at(x, y) {
				continue
			}
			switch {
			case p.blink:
				put(x, y, furColor(x, y))
			case onEdge(&eyes, x, y):
				put(x, y, outlineColor)
			case float64(y) > eyeCY+3.5:
				put(x, y, eyeShade)
			default:
				put(x, y, eyeWhite)
			}
		}
	}
	for _, side := range []struct{ eyeX, pupilX float64 }{{leftEyeCX, 13.5}, {rightEyeCX, 34.5}} {
		cx, cy := int(side.eyeX), int(math.Floor(eyeCY))
		if p.blink {
			// にっこり閉じた目 (^)。
			for dx := -2; dx <= 2; dx++ {
				put(cx+dx, cy, outlineColor)
			}
			for _, dx := range []int{-4, -3, 3, 4} {
				put(cx+dx, cy+1, outlineColor)
			}
			put(cx-5, cy+2, outlineColor)
			put(cx+5, cy+2, outlineColor)
			continue
		}
		// 瞳は丸い 4×4 ドット。左上に光を 1 ドット入れる。
		px, py := int(side.pupilX), cy+3
		for dy := -2; dy <= 1; dy++ {
			for dx := -2; dx <= 1; dx++ {
				corner := (dx == -2 || dx == 1) && (dy == -2 || dy == 1)
				if !corner {
					put(px+dx, py+dy, pupilColor)
				}
			}
		}
		put(px-1, py-1, eyeWhite)
	}
	return img, set
}

// ----- 背景・文字・ロゴ -----

// drawText は mask に文字列 s を、ベースライン baseline で左右中央に描く (超解像度)。
func drawText(mask *image.Alpha, ttf []byte, size, baseline float64, s string) error {
	f, err := opentype.Parse(ttf)
	if err != nil {
		return err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size * supersample,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return err
	}
	defer face.Close()

	advance := font.MeasureString(face, s)
	d := font.Drawer{
		Dst:  mask,
		Src:  image.Opaque,
		Face: face,
		Dot:  fixed.P((width*supersample-advance.Round())/2, int(baseline*supersample)),
	}
	d.DrawString(s)
	return nil
}

// logo はロゴ画像の各画素の濃さ (0-1) と、ロゴの色、描かれている範囲。
type logo struct {
	coverage []float64
	stride   int
	bounds   image.Rectangle
	color    rgb
}

func luminance(c color.NRGBA) float64 {
	return (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
}

// loadLogo はロゴ画像を読み込む。最も濃い不透明な色をロゴの色とし、
// 各画素がその色にどれだけ近いか (白地や透明に近いほど 0) を濃さとする。
func loadLogo() (logo, error) {
	img, err := png.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return logo{}, err
	}
	b := img.Bounds()
	at := func(x, y int) color.NRGBA {
		return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	}

	ink := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c := at(x, y); c.A == 255 && luminance(c) < luminance(ink) {
				ink = c
			}
		}
	}
	inkLum := luminance(ink)

	l := logo{
		coverage: make([]float64, b.Dx()*b.Dy()),
		stride:   b.Dx(),
		color:    rgb{float64(ink.R) / 255, float64(ink.G) / 255, float64(ink.B) / 255},
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := at(x, y)
			cov := float64(c.A) / 255 * clamp01((1-luminance(c))/(1-inkLum))
			lx, ly := x-b.Min.X, y-b.Min.Y
			l.coverage[ly*l.stride+lx] = cov
			if cov > 0.05 {
				l.bounds = l.bounds.Union(image.Rect(lx, ly, lx+1, ly+1))
			}
		}
	}
	return l, nil
}

// sample は元画像の座標 (sx, sy) の濃さを双線形補間で返す。
func (l logo) sample(sx, sy float64) float64 {
	h := len(l.coverage) / l.stride
	at := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= l.stride || y >= h {
			return 0
		}
		return l.coverage[y*l.stride+x]
	}
	x0, y0 := math.Floor(sx-0.5), math.Floor(sy-0.5)
	fx, fy := sx-0.5-x0, sy-0.5-y0
	ix, iy := int(x0), int(y0)
	top := at(ix, iy)*(1-fx) + at(ix+1, iy)*fx
	bottom := at(ix, iy+1)*(1-fx) + at(ix+1, iy+1)*fx
	return top*(1-fy) + bottom*fy
}

// at は画面上の位置 (x, y) におけるロゴの濃さを返す。ロゴは幅 logoWidth で左右中央に置く。
func (l logo) at(x, y float64) float64 {
	s := logoWidth / float64(l.bounds.Dx())
	x0 := (width - logoWidth) / 2
	y0 := logoCenterY - float64(l.bounds.Dy())*s/2
	return l.sample(float64(l.bounds.Min.X)+(x-x0)/s, float64(l.bounds.Min.Y)+(y-y0)/s)
}

// backdrop は背景色に、ドット模様と足元の影を重ねた色を返す。
func backdrop(x, y float64) rgb {
	c := bgColor
	// 1 行ごとに半分ずらした格子状に点を置く。
	row := int(y) / bgDotSpacing
	shift := (row % 2) * bgDotSpacing / 2
	if (int(x)+shift)%bgDotSpacing < bgDotSize && int(y)%bgDotSpacing < bgDotSize {
		c = bgDotColor
	}
	shadowY := float64(spriteTop + 44*dotSize)
	if math.Hypot((x-width/2)/shadowRadiusX, (y-shadowY)/shadowRadiusY) < 1 {
		c = shadowColor
	}
	return c
}

// renderBackground は動かない部分 (背景・文字・ロゴ) を、画素ごとの色で返す。
func renderBackground() ([]rgb, error) {
	const sw, sh = width * supersample, height * supersample
	titleMask := image.NewAlpha(image.Rect(0, 0, sw, sh))
	err := drawText(titleMask, gobold.TTF, titleSize, titleBaseline, title)
	if err != nil {
		return nil, err
	}
	creditMask := image.NewAlpha(image.Rect(0, 0, sw, sh))
	for i, line := range creditLines {
		err := drawText(creditMask, goregular.TTF, creditSize, creditBaseline+float64(i)*creditLineHeight, line)
		if err != nil {
			return nil, err
		}
	}
	lg, err := loadLogo()
	if err != nil {
		return nil, err
	}

	bg := make([]rgb, width*height)
	for py := 0; py < height; py++ {
		for px := 0; px < width; px++ {
			var sum rgb
			for sy := 0; sy < supersample; sy++ {
				for sx := 0; sx < supersample; sx++ {
					ix, iy := px*supersample+sx, py*supersample+sy
					x := (float64(ix) + 0.5) / supersample
					y := (float64(iy) + 0.5) / supersample
					c := backdrop(float64(px), float64(py))
					c = mix(c, lg.color, lg.at(x, y))
					c = mix(c, titleColor, float64(titleMask.AlphaAt(ix, iy).A)/255)
					c = mix(c, creditColor, float64(creditMask.AlphaAt(ix, iy).A)/255)
					sum = rgb{sum.r + c.r, sum.g + c.g, sum.b + c.b}
				}
			}
			const n = supersample * supersample
			bg[py*width+px] = rgb{sum.r / n, sum.g / n, sum.b / n}
		}
	}
	return bg, nil
}

// renderFrame はコマ frame を回転前の向きで描き、画素ごとの色を返す。
func renderFrame(bg []rgb, frame int) []rgb {
	out := make([]rgb, len(bg))
	copy(out, bg)
	p := poseAt(frame)
	img, set := drawGopher(p)
	top := spriteTop - p.bob*dotSize
	for dy := 0; dy < dotH; dy++ {
		for dx := 0; dx < dotW; dx++ {
			if !set[dy][dx] {
				continue
			}
			for y := top + dy*dotSize; y < top+(dy+1)*dotSize; y++ {
				for x := spriteLeft + dx*dotSize; x < spriteLeft+(dx+1)*dotSize; x++ {
					out[y*width+x] = img[dy][dx]
				}
			}
		}
	}
	return out
}

// ----- 回転・変換・出力 -----

// sourcePixel は、液晶上の画素 (nx, ny) に表示すべき回転前の画素の位置を返す。
// rotate は時計回りの回転角 (0, 90, 180, 270)。
func sourcePixel(nx, ny, rotate int) (int, int) {
	switch rotate {
	case 90:
		return ny, width - 1 - nx
	case 180:
		return width - 1 - nx, height - 1 - ny
	case 270:
		return height - 1 - ny, nx
	default:
		return nx, ny
	}
}

// quantize は 0-1 の値を 0-levels の段階に丸める。ドット絵の色がにじまないよう、ディザはかけない。
func quantize(v float64, levels int) int {
	return int(math.Round(clamp01(v) * float64(levels)))
}

// toRGB565 は回転前のコマを液晶の向きに回転させ、RGB565 に変換する。
func toRGB565(frame []rgb, rotate int) []uint16 {
	out := make([]uint16, width*height)
	for ny := 0; ny < height; ny++ {
		for nx := 0; nx < width; nx++ {
			lx, ly := sourcePixel(nx, ny, rotate)
			c := frame[ly*width+lx]
			out[ny*width+nx] = uint16(quantize(c.r, 31)<<11 | quantize(c.g, 63)<<5 | quantize(c.b, 31))
		}
	}
	return out
}

func rgb565ToColor(v uint16) color.RGBA {
	return color.RGBA{
		R: uint8(int(v>>11) * 255 / 31),
		G: uint8(int(v>>5&0x3f) * 255 / 63),
		B: uint8(int(v&0x1f) * 255 / 31),
		A: 255,
	}
}

// changedRegion は、いずれかのコマで 1 コマ目と異なる画素を含む範囲を返す。
func changedRegion(frames [][]uint16) image.Rectangle {
	r := image.Rectangle{}
	for _, f := range frames[1:] {
		for i, v := range f {
			if v != frames[0][i] {
				x, y := i%width, i/width
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r.Inset(-regionMargin).Intersect(image.Rect(0, 0, width, height))
}

func appendRegion(buf []byte, frame []uint16, r image.Rectangle) []byte {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			v := frame[y*width+x]
			buf = append(buf, byte(v>>8), byte(v))
		}
	}
	return buf
}

func writeGoFile(path, pkg string, r image.Rectangle) error {
	src := fmt.Sprintf(`// Code generated by tools/genimage; DO NOT EDIT.

package %s

import "time"

// frames.rgb565 の各コマを描く範囲とコマ数、コマの間隔。
const (
	spriteX       = %d
	spriteY       = %d
	spriteW       = %d
	spriteH       = %d
	numFrames     = %d
	frameInterval = %d * time.Millisecond
)
`, pkg, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), numFrames, frameDelayMs)
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0o644)
}

// toPreview は液晶向きのコマを回転前の向きに戻した画像を返す。
func toPreview(f []uint16, rotate int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for ny := 0; ny < height; ny++ {
		for nx := 0; nx < width; nx++ {
			lx, ly := sourcePixel(nx, ny, rotate)
			img.SetRGBA(lx, ly, rgb565ToColor(f[ny*width+nx]))
		}
	}
	return img
}

// writePreview は回転前の向きで、液晶と同じ色数に落とした GIF を書き出す。
func writePreview(path string, frames [][]uint16, rotate int) error {
	anim := &gif.GIF{}
	for _, f := range frames {
		img := toPreview(f, rotate)
		pal := image.NewPaletted(img.Bounds(), palette.Plan9)
		draw.FloydSteinberg.Draw(pal, img.Bounds(), img, image.Point{})
		anim.Image = append(anim.Image, pal)
		anim.Delay = append(anim.Delay, frameDelayMs/10)
	}
	var buf bytes.Buffer
	err := gif.EncodeAll(&buf, anim)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// writeSheet は全コマを回転前の向きで並べた PNG を書き出す。
func writeSheet(path string, frames [][]uint16, rotate int) error {
	const cols = 6
	rows := (len(frames) + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, width*cols, height*rows))
	for i, f := range frames {
		at := image.Pt(i%cols*width, i/cols*height)
		draw.Draw(sheet, image.Rectangle{Min: at, Max: at.Add(image.Pt(width, height))}, toPreview(f, rotate), image.Point{}, draw.Src)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, sheet)
}

func run() error {
	dir := flag.String("dir", ".", "output directory")
	pkg := flag.String("pkg", "main", "package name of the generated Go file")
	rotate := flag.Int("rotate", 90, "clockwise rotation for the display (0, 90, 180, 270)")
	sheetPath := flag.String("sheet", "", "optional path of a PNG contact sheet of all frames")
	flag.Parse()
	if *rotate%90 != 0 || *rotate < 0 || *rotate >= 360 {
		return fmt.Errorf("invalid -rotate %d", *rotate)
	}

	bg, err := renderBackground()
	if err != nil {
		return err
	}
	frames := make([][]uint16, numFrames)
	for i := range frames {
		frames[i] = toRGB565(renderFrame(bg, i), *rotate)
	}
	region := changedRegion(frames)

	background := appendRegion(nil, frames[0], image.Rect(0, 0, width, height))
	err = os.WriteFile(filepath.Join(*dir, "background.rgb565"), background, 0o644)
	if err != nil {
		return err
	}
	sprites := make([]byte, 0, numFrames*region.Dx()*region.Dy()*bytesPerPixel)
	for _, f := range frames {
		sprites = appendRegion(sprites, f, region)
	}
	err = os.WriteFile(filepath.Join(*dir, "frames.rgb565"), sprites, 0o644)
	if err != nil {
		return err
	}
	err = writeGoFile(filepath.Join(*dir, "anim_gen.go"), *pkg, region)
	if err != nil {
		return err
	}
	err = writePreview(filepath.Join(*dir, "preview.gif"), frames, *rotate)
	if err != nil {
		return err
	}
	if *sheetPath != "" {
		return writeSheet(*sheetPath, frames, *rotate)
	}
	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}
