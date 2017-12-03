package main

import (
	"testing"

	"github.com/gonutz/d3dmath"
)

func TestPlaneLineIntersection(t *testing.T) {
	x := planeLineIntersection(
		[3]d3dmath.Vec3{
			{0, 0, 0},
			{1, 0, 0},
			{0, 0, 1},
		},
		[2]d3dmath.Vec3{
			{0, 0, 0},
			{0, 1, 0},
		},
	).String()
	if x != "(0.00 0.00 0.00)" {
		t.Errorf("want 0,0,0 but have %s", x)
	}
}
