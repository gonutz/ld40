// +build ignore

package main

import (
	"math/rand"

	"github.com/gonutz/img"
)

func main() {
	img.Run(func(p *img.Pixel) {
		p.R, p.G, p.B, p.A = 0, uint8(60+rand.Intn(30)), 0, 255
	})
}
