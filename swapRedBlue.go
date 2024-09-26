//go:build ignore
// +build ignore

package main

import "github.com/gonutz/img"

func main() {
	img.Run(func(p *img.Pixel) {
		p.R, p.B = p.B, p.R
	})
}
