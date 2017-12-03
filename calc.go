package main

import "github.com/gonutz/d3dmath"

// NOTE this assumes that plane points are not colinear, line points are not
// colinear and that the line is not parallel to the plane
func planeLineIntersection(plane [3]d3dmath.Vec3, line [2]d3dmath.Vec3) d3dmath.Vec3 {
	// determine plane equation: a*x + b*y + c*z - d = 0
	// where (a, b, c) is the plane's normal vector
	n := plane[1].Sub(plane[0]).Cross(
		plane[2].Sub(plane[0])).Normalized() // TODO is Normalized needed?
	// plane equation: n[0]*x + n[1]*y + n[2]*z - d = 0
	//             <=> d = n[0]*x + n[1]*y + n[2]*z which is the dot-product
	// to find d, plug in any point p on the plane
	p := plane[0]
	d := n.Dot(p)
	// line parameterization: l(t) = l1 + t*(l2 - l1)
	//                             = a + t*b
	a, b := line[0], line[1].Sub(line[0])
	// the parametric line form describes all points on the line
	// the plane equation is a test for inclusion of points in the plane
	t := (n.Dot(a) - d) / (-n.Dot(b))
	return a.Add(b.MulScalar(t))
}
