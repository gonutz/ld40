package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/gonutz/blob"
	"github.com/gonutz/d3d9"
	"github.com/gonutz/d3dmath"
	"github.com/gonutz/payload"
	"github.com/gonutz/w32"
	"github.com/gonutz/win"
)

var windowW, windowH = 480, 320

func main() {
	defer handlePanics()

	runtime.LockOSThread()

	win.HideConsoleWindow()

	// the initial values for windowW and windowH describe the desired window
	// client size, not the overall window size (which includes borders and a
	// title bar) so initially calculate what the window size should be to get a
	// draw area of the desired size
	r := w32.RECT{Left: 0, Top: 0, Right: int32(windowW), Bottom: int32(windowH)}
	if w32.AdjustWindowRect(&r, w32.WS_OVERLAPPEDWINDOW, false) {
		windowW = int(r.Width())
		windowH = int(r.Height())
	}

	var oldWindowPos w32.WINDOWPLACEMENT
	toggleFullscreen := func(window w32.HWND) {
		if win.IsFullscreen(window) {
			win.DisableFullscreen(window, oldWindowPos)
			w32.ShowCursor(false)
		} else {
			oldWindowPos = win.EnableFullscreen(window)
		}
	}

	active := false

	computeScreenCenter := func(window w32.HWND) {
		if !active {
			return
		}
		gameState.centerX, gameState.centerY = w32.ClientToScreen(window, windowW/2, windowH/2)
		gameState.mouseX = gameState.centerX
		gameState.mouseY = gameState.centerY
		w32.SetCursorPos(gameState.centerX, gameState.centerY)
	}

	window, err := win.NewWindow(
		w32.CW_USEDEFAULT,
		w32.CW_USEDEFAULT,
		windowW,
		windowH,
		"LD40window",
		func(window w32.HWND, msg uint32, w, l uintptr) uintptr {
			switch msg {
			case w32.WM_MOUSEMOVE:
				x := int((uint(l)) & 0xFFFF)
				y := int((uint(l) >> 16) & 0xFFFF)
				gameState.mouseX, gameState.mouseY = w32.ClientToScreen(window, x, y)
				return 0
			case w32.WM_LBUTTONDOWN:
				gameState.keyShootDown = true
				return 0
			case w32.WM_LBUTTONUP:
				gameState.keyShootDown = false
				return 0
			case w32.WM_KEYDOWN:
				if l&(1<<30) != 0 {
					// if the key was down before, ignore it, no auto-repeat
					return 0
				}
				switch w {
				case 'W':
					gameState.keyForwardDown = true
				case 'S':
					gameState.keyBackwardDown = true
				case 'A':
					gameState.keyLeftDown = true
				case 'D':
					gameState.keyRightDown = true
				case w32.VK_SHIFT:
					gameState.keyRunDown = true
				case w32.VK_CONTROL:
					gameState.keySneakDown = true
				case w32.VK_SPACE:
					gameState.keyJumpDown = true
				case w32.VK_ESCAPE:
					win.CloseWindow(window)
				case w32.VK_F11:
					toggleFullscreen(window)
				}
				return 0
			case w32.WM_KEYUP:
				switch w {
				case 'W':
					gameState.keyForwardDown = false
				case 'S':
					gameState.keyBackwardDown = false
				case 'A':
					gameState.keyLeftDown = false
				case 'D':
					gameState.keyRightDown = false
				case w32.VK_SHIFT:
					gameState.keyRunDown = false
				case w32.VK_CONTROL:
					gameState.keySneakDown = false
				case w32.VK_SPACE:
					gameState.keyJumpDown = false
				}
				return 0
			case w32.WM_SIZE:
				windowW = int((uint(l)) & 0xFFFF)
				windowH = int((uint(l) >> 16) & 0xFFFF)
				computeScreenCenter(window)
				return 0
			case w32.WM_MOVE:
				computeScreenCenter(window)
				return 0
			case w32.WM_ACTIVATE:
				active = w != 0
				return 0
			case w32.WM_DESTROY:
				w32.PostQuitMessage(0)
				return 0
			default:
				return w32.DefWindowProc(window, msg, w, l)
			}
		},
	)
	check(err)
	w32.SetWindowText(window, "LD 40 - The more you have, the worse it is")
	win.SetIconFromExe(window, 10)
	toggleFullscreen(window)
	computeScreenCenter(window)

	d3d, err := d3d9.Create(d3d9.SDK_VERSION)
	check(err)
	defer d3d.Release()

	var createFlags uint32 = d3d9.CREATE_SOFTWARE_VERTEXPROCESSING
	caps, err := d3d.GetDeviceCaps(d3d9.ADAPTER_DEFAULT, d3d9.DEVTYPE_HAL)
	if err == nil &&
		caps.DevCaps&d3d9.DEVCAPS_HWTRANSFORMANDLIGHT != 0 {
		createFlags = d3d9.CREATE_HARDWARE_VERTEXPROCESSING
	}
	presentParameters := d3d9.PRESENT_PARAMETERS{
		Windowed:               1,
		HDeviceWindow:          d3d9.HWND(window),
		SwapEffect:             d3d9.SWAPEFFECT_COPY, // so Present can use rects
		BackBufferWidth:        uint32(windowW),
		BackBufferHeight:       uint32(windowH),
		BackBufferFormat:       d3d9.FMT_UNKNOWN,
		BackBufferCount:        1,
		EnableAutoDepthStencil: 1,
		AutoDepthStencilFormat: d3d9.FMT_D24X8,
	}
	device, actualPP, err := d3d.CreateDevice(
		d3d9.ADAPTER_DEFAULT,
		d3d9.DEVTYPE_HAL,
		d3d9.HWND(window),
		createFlags,
		presentParameters,
	)
	presentParameters = actualPP
	check(err)
	defer device.Release()

	// NOTE comment this to switch between starting in fullscreen or not
	//      this has to come after the presentParameters so the back buffer has
	//      the size of the whole screen
	//toggleFullscreen(window)

	setRenderState := func(device *d3d9.Device) {
		check(device.SetRenderState(d3d9.RS_CULLMODE, d3d9.CULL_CW))
		check(device.SetRenderState(d3d9.RS_ALPHABLENDENABLE, 0))
		check(device.SetRenderState(d3d9.RS_ZENABLE, d3d9.ZB_TRUE))
		// TODO this is strange: usually you use d3d9.CMP_LESS here but that
		// does not render anything for me. instead we have to flip the near and
		// far plane values in the d3dmath.Perspective matrix (further down) and
		// use GREATER here. why is that?!
		check(device.SetRenderState(d3d9.RS_ZFUNC, d3d9.CMP_GREATER))
	}
	setRenderState(device)

	createGeometry(device)
	defer destroyGeometry()

	deviceIsLost := false
	const frameDelay = time.Second / 60
	lastFrame := time.Now().Add(-frameDelay)
	win.RunMainGameLoop(func() {
		now := time.Now()
		if now.Sub(lastFrame) < frameDelay {
			time.Sleep(time.Nanosecond)
		} else {
			lastFrame = now

			if active {
				updateGame()
			}

			if deviceIsLost {
				_, err = device.Reset(presentParameters)
				if err == nil {
					deviceIsLost = false
					setRenderState(device)
					// TODO reset vertex buffers
					// TODO reset vertex declarations
					// TODO reset textures
				}
			}

			if !deviceIsLost {
				check(device.SetViewport(
					d3d9.VIEWPORT{
						X:      0,
						Y:      0,
						Width:  uint32(windowW),
						Height: uint32(windowH),
						MinZ:   0,
						MaxZ:   1,
					},
				))

				check(device.Clear(
					nil,
					d3d9.CLEAR_TARGET+d3d9.CLEAR_ZBUFFER,
					d3d9.ColorRGB(255, 0, 0),
					1,
					0,
				))
				check(device.BeginScene())
				renderGeometry(device)
				check(device.EndScene())
				r := &d3d9.RECT{0, 0, int32(windowW), int32(windowH)}
				presentErr := device.Present(r, r, 0, nil)
				if presentErr != nil {
					if presentErr.Code() == d3d9.ERR_DEVICELOST {
						deviceIsLost = true
					} else {
						check(presentErr)
					}
				}
			}
		}
	})
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// handle all panics in the program and open a log file after a crash
func handlePanics() {
	if err := recover(); err != nil {
		// in case of a panic, create a message with the current stack
		msg := fmt.Sprintf("panic: %v\nstack:\n\n%s\n", err, debug.Stack())

		// print it to stdout
		fmt.Println(msg)

		// write it to a log file
		filename := filepath.Join(
			os.Getenv("APPDATA"),
			"ld40_log_"+time.Now().Format("2006_01_02__15_04_05")+".txt",
		)
		ioutil.WriteFile(filename, []byte(msg), 0777)

		// open the log file with the default text viewer
		exec.Command("cmd", "/C", filename).Start()

		// pop up a message box
		w32.MessageBox(
			0,
			msg,
			"The program crashed",
			w32.MB_OK|w32.MB_ICONERROR|w32.MB_TOPMOST,
		)
	}
}

var (
	// d3d9 assets
	uniColorVS    *d3d9.VertexShader
	uniColorPS    *d3d9.PixelShader
	uniColorDecl  *d3d9.VertexDeclaration
	texVS         *d3d9.VertexShader
	texPS         *d3d9.PixelShader
	texDecl       *d3d9.VertexDeclaration
	texLitVS      *d3d9.VertexShader
	texLitPS      *d3d9.PixelShader
	texLitDecl    *d3d9.VertexDeclaration
	vertices      *d3d9.VertexBuffer
	triangles     *d3d9.VertexBuffer
	texture       *d3d9.Texture
	sky           *d3d9.Texture
	skyVertices   *d3d9.VertexBuffer
	floor         *d3d9.Texture
	floorVertices *d3d9.VertexBuffer
	square        *d3d9.VertexBuffer
)

func destroyGeometry() {
	if uniColorVS != nil {
		uniColorVS.Release()
		uniColorVS = nil
	}
	if uniColorPS != nil {
		uniColorPS.Release()
		uniColorPS = nil
	}
	if uniColorDecl != nil {
		uniColorDecl.Release()
		uniColorDecl = nil
	}
	if texVS != nil {
		texVS.Release()
		texVS = nil
	}
	if texPS != nil {
		texPS.Release()
		texPS = nil
	}
	if texDecl != nil {
		texDecl.Release()
		texDecl = nil
	}
	if texLitVS != nil {
		texLitVS.Release()
		texLitVS = nil
	}
	if texLitPS != nil {
		texLitPS.Release()
		texLitPS = nil
	}
	if texLitDecl != nil {
		texLitDecl.Release()
		texLitDecl = nil
	}
	if vertices != nil {
		vertices.Release()
		vertices = nil
	}
	if triangles != nil {
		triangles.Release()
		triangles = nil
	}
	if texture != nil {
		texture.Release()
		texture = nil
	}
	if sky != nil {
		sky.Release()
		sky = nil
	}
	if skyVertices != nil {
		skyVertices.Release()
		skyVertices = nil
	}
	if floor != nil {
		floor.Release()
		floor = nil
	}
	if floorVertices != nil {
		floorVertices.Release()
		floorVertices = nil
	}
	if square != nil {
		square.Release()
		square = nil
	}
}

func createGeometry(device *d3d9.Device) {
	var err error

	uniColorVS, err = device.CreateVertexShaderFromBytes(vertexShader_uniform_color)
	check(err)
	uniColorPS, err = device.CreatePixelShaderFromBytes(pixelShader_uniform_color)
	check(err)

	uniColorDecl, err = device.CreateVertexDeclaration(
		[]d3d9.VERTEXELEMENT{
			d3d9.VERTEXELEMENT{
				Stream:     0,
				Offset:     0,
				Type:       d3d9.DECLTYPE_FLOAT3,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_POSITION,
				UsageIndex: 0,
			},
			d3d9.DeclEnd(),
		},
	)
	check(err)

	vertices = createVertexBuffer(device, []float32{
		0, -0.5, 0,
		1, -0.5, 0,
		0, 0.5, 0,

		5 + 0, -0.5, 0,
		5 + 1, -0.5, 0,
		5 + 0, 0.5, 0,
	})

	skyVertices = createVertexBuffer(device, []float32{
		// top
		-1, 1, 1,
		0, 0.5,
		1, 1, 1,
		1.0 / 3, 0.5,
		-1, 1, -1,
		0, 0,

		-1, 1, -1,
		0, 0,
		1, 1, 1,
		1.0 / 3, 0.5,
		1, 1, -1,
		1.0 / 3, 0,

		// bottom
		-1, -1, -1,
		1.0 / 3, 0.5,
		1, -1, -1,
		2.0 / 3, 0.5,
		-1, -1, 1,
		1.0 / 3, 0,

		-1, -1, 1,
		1.0 / 3, 0,
		1, -1, -1,
		2.0 / 3, 0.5,
		1, -1, 1,
		2.0 / 3, 0,

		// left
		-1, -1, 1,
		0, 1,
		1, -1, 1,
		1.0 / 3, 1,
		-1, 1, 1,
		0, 0.5,

		-1, 1, 1,
		0, 0.5,
		1, -1, 1,
		1.0 / 3, 1,
		1, 1, 1,
		1.0 / 3, 0.5,

		// front
		-1, -1, -1,
		2.0 / 3, 0.5,
		-1, -1, 1,
		1, 0.5,
		-1, 1, -1,
		2.0 / 3, 0,

		-1, 1, -1,
		2.0 / 3, 0,
		-1, -1, 1,
		1, 0.5,
		-1, 1, 1,
		1, 0,

		// right
		1, -1, 1,
		1.0 / 3, 1,
		1, -1, -1,
		2.0 / 3, 1,
		1, 1, 1,
		1.0 / 3, 0.5,

		1, 1, 1,
		1.0 / 3, 0.5,
		1, -1, -1,
		2.0 / 3, 1,
		1, 1, -1,
		2.0 / 3, 0.5,

		// back
		1, -1, -1,
		2.0 / 3, 1,
		-1, -1, -1,
		1, 1,
		1, 1, -1,
		2.0 / 3, 0.5,

		1, 1, -1,
		2.0 / 3, 0.5,
		-1, -1, -1,
		1, 1,
		-1, 1, -1,
		1, 0.5,
	})

	texVS, err = device.CreateVertexShaderFromBytes(vertexShader_texture)
	check(err)
	texPS, err = device.CreatePixelShaderFromBytes(pixelShader_texture)
	check(err)

	texDecl, err = device.CreateVertexDeclaration(
		[]d3d9.VERTEXELEMENT{
			d3d9.VERTEXELEMENT{
				Stream:     0,
				Offset:     0,
				Type:       d3d9.DECLTYPE_FLOAT3,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_POSITION,
				UsageIndex: 0,
			},
			d3d9.VERTEXELEMENT{
				Stream:     0,
				Offset:     3 * 4,
				Type:       d3d9.DECLTYPE_FLOAT2,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_TEXCOORD,
				UsageIndex: 0,
			},
			d3d9.DeclEnd(),
		},
	)
	check(err)

	texLitVS, err = device.CreateVertexShaderFromBytes(vertexShader_texture_lit)
	check(err)
	texLitPS, err = device.CreatePixelShaderFromBytes(pixelShader_texture_lit)
	check(err)
	texLitDecl, err = device.CreateVertexDeclaration(
		[]d3d9.VERTEXELEMENT{
			d3d9.VERTEXELEMENT{
				Stream:     0,
				Offset:     0,
				Type:       d3d9.DECLTYPE_FLOAT3,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_POSITION,
				UsageIndex: 0,
			},
			d3d9.VERTEXELEMENT{
				Stream:     0,
				Offset:     3 * 4,
				Type:       d3d9.DECLTYPE_FLOAT3,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_NORMAL,
				UsageIndex: 0,
			},
			d3d9.VERTEXELEMENT{
				Stream:     0,
				Offset:     (3 + 3) * 4,
				Type:       d3d9.DECLTYPE_FLOAT2,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_TEXCOORD,
				UsageIndex: 0,
			},
			d3d9.DeclEnd(),
		},
	)
	check(err)

	triangles = createVertexBuffer(device, []float32{
		-3 + 0, 0, 0,
		-3 + 0, 1,
		-3 + 1, 0, 0,
		-3 + 1, 1,
		-3 + 0, 1, 0,
		-3 + 0, 0,

		5 + 0, 0, 0,
		0, 0,
		5 + 1, 0, 0,
		0, 1,
		5 + 0, 1, 0,
		1, 0,
	})

	square = createVertexBuffer(device, []float32{
		0, 0, 0,
		1, 0, 0,
		0, 0, 1,

		0, 0, 1,
		1, 0, 0,
		1, 0, 1,
	})

	texture = loadTexture(device, "texture.png")
	sky = loadTexture(device, "sky.png")

	floor = loadTexture(device, "floor.png")

	// height field from black and white image
	ground = loadHeightField("heights.png")
	ground.scale = d3dmath.Vec3{0.25, 1.3, 0.25}

	floorVertices = createVertexBuffer(device, heightFieldVertices(ground))
}

func loadHeightField(path string) heightField {
	const scale = 1.0 / 127
	var field heightField

	img := loadPng(path)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w != h {
		panic("can only handle square height fields right now")
	}
	heights := make([]float32, 0, w*h)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			heights = append(heights, (float32(img.RGBAAt(x, y).R)-127)*scale)
		}
	}

	// slice the linear array into a 2D array for the result
	field.heights = make([][]float32, h)
	for i := range field.heights {
		field.heights[i] = heights[i*w : (i+1)*w]
	}

	return field
}

type heightField struct {
	heights [][]float32
	scale   d3dmath.Vec3 // the height field is first offset, then scaled
}

func (h heightField) size() int {
	return len(h.heights) - 1
}

func (h heightField) offset() (x, y, z float32) {
	x = -float32(h.size()) / 2
	z = x
	return
}

func (h heightField) modelTransform() d3dmath.Mat4 {
	return d3dmath.Mul4(
		d3dmath.Translate(h.offset()),
		d3dmath.ScaleV(h.scale),
	)
}

func heightAt(x, z float32, h heightField) float32 {
	x /= h.scale[0]
	z /= h.scale[2]
	dx, _, dz := h.offset()
	x -= dx
	z -= dz
	size := float32(h.size())
	if x < 0 || z < 0 || x >= size || z >= size {
		return 0
	}
	/* at this point x,z are in tile coordinates
	        z
	        ^
	        |
	        |
	   03 13|23 33
	   02 12|22 32
	--------+----------> x
	   01 11|21 31
	   00 10|20 30
	        |
	        |
	*/
	ix, iz := int(x), int(z)
	fx, fz := x-float32(ix), z-float32(iz)
	onLeftTriangle := 1.0-fx > fz

	heightBottomLeft := h.heights[h.size()-iz][ix]
	heightTopLeft := h.heights[h.size()-iz-1][ix]
	heightBottomRight := h.heights[h.size()-iz][ix+1]
	heightTopRight := h.heights[h.size()-iz-1][ix+1]
	triangle := [3]d3dmath.Vec3{
		d3dmath.Vec3{1, heightBottomRight, 0},
		d3dmath.Vec3{0, heightTopLeft, 1},
	}
	if onLeftTriangle {
		triangle[2] = d3dmath.Vec3{0, heightBottomLeft, 0}
	} else {
		triangle[2] = d3dmath.Vec3{1, heightTopRight, 1}
	}
	line := [2]d3dmath.Vec3{
		d3dmath.Vec3{fx, 0, fz},
		d3dmath.Vec3{fx, 1, fz},
	}
	p := planeLineIntersection(triangle, line)
	return p[1] * h.scale[1]
}

var ground heightField

func heightFieldVertices(heightField heightField) []float32 {
	size := len(heightField.heights) - 1
	h := make([]float32, 0, size*size*6*(3+3+2)) // 2 triangles: pos, normal, uv
	for z := 0; z < size; z++ {
		for x := 0; x < size; x++ {
			fx, fz := float32(x), float32(z)
			i, j := size-z, x
			y1 := heightField.heights[i][j]
			y2 := heightField.heights[i][j+1]
			y3 := heightField.heights[i-1][j]
			y4 := heightField.heights[i-1][j+1]
			if z == 0 || x == 0 || z == size-1 || x == size-1 {
				// at the edges the normals are set to 0,1,0
				h = append(h, []float32{
					fx, y1, fz,
					0, 1, 0,
					0, 1,
					fx + 1, y2, fz,
					0, 1, 0,
					1, 1,
					fx, y3, fz + 1,
					0, 1, 0,
					0, 0,

					fx, y3, fz + 1,
					0, 1, 0,
					0, 0,
					fx + 1, y2, fz,
					0, 1, 0,
					1, 1,
					fx + 1, y4, fz + 1,
					0, 1, 0,
					1, 0,
				}...)
			} else {
				n := [2 + 3 + 4 + 3 + 2]d3dmath.Vec3{
					{fx + 0, heightField.heights[i+1][j+0], fz - 1},
					{fx - 1, heightField.heights[i+0][j-1], fz + 0},
					{fx + 1, heightField.heights[i+1][j+1], fz - 1},
					{fx + 0, heightField.heights[i+0][j+0], fz + 0},
					{fx - 1, heightField.heights[i-1][j-1], fz + 1},
					{fx + 2, heightField.heights[i+1][j+2], fz - 1},
					{fx + 1, heightField.heights[i+0][j+1], fz + 0},
					{fx + 0, heightField.heights[i-1][j+0], fz + 1},
					{fx - 1, heightField.heights[i-2][j-1], fz + 2},
					{fx + 2, heightField.heights[i+0][j+2], fz + 0},
					{fx + 1, heightField.heights[i-1][j+1], fz + 1},
					{fx + 0, heightField.heights[i-2][j+0], fz + 2},
					{fx + 2, heightField.heights[i-1][j+2], fz + 1},
					{fx + 1, heightField.heights[i-2][j+1], fz + 2},
				}
				for i := range n {
					n[i][0] *= heightField.scale[0]
					n[i][1] *= heightField.scale[1]
					n[i][2] *= heightField.scale[2]
				}
				normals := [16]d3dmath.Vec3{
					n[3].Sub(n[0]).Cross(n[2].Sub(n[0])),
					n[0].Sub(n[3]).Cross(n[1].Sub(n[3])),
					n[4].Sub(n[1]).Cross(n[3].Sub(n[1])),
					n[6].Sub(n[2]).Cross(n[5].Sub(n[2])),
					n[2].Sub(n[6]).Cross(n[3].Sub(n[6])),
					n[7].Sub(n[3]).Cross(n[6].Sub(n[3])),
					n[3].Sub(n[7]).Cross(n[4].Sub(n[7])),
					n[8].Sub(n[4]).Cross(n[7].Sub(n[4])),
					n[5].Sub(n[9]).Cross(n[6].Sub(n[9])),
					n[10].Sub(n[6]).Cross(n[9].Sub(n[6])),
					n[6].Sub(n[10]).Cross(n[7].Sub(n[10])),
					n[11].Sub(n[7]).Cross(n[10].Sub(n[7])),
					n[7].Sub(n[11]).Cross(n[8].Sub(n[11])),
					n[9].Sub(n[12]).Cross(n[10].Sub(n[12])),
					n[13].Sub(n[10]).Cross(n[12].Sub(n[10])),
					n[10].Sub(n[13]).Cross(n[11].Sub(n[13])),
				}
				n00 := d3dmath.AddVec3(
					normals[0], normals[1], normals[2],
					normals[4], normals[5], normals[6],
				).Normalized()
				n10 := d3dmath.AddVec3(
					normals[3], normals[4], normals[5],
					normals[8], normals[9], normals[10],
				).Normalized()
				n01 := d3dmath.AddVec3(
					normals[5], normals[6], normals[7],
					normals[10], normals[11], normals[12],
				).Normalized()
				n11 := d3dmath.AddVec3(
					normals[9], normals[10], normals[11],
					normals[13], normals[14], normals[15],
				).Normalized()
				h = append(h, []float32{
					fx, y1, fz,
					n00[0], n00[1], n00[2],
					0, 1,
					fx + 1, y2, fz,
					n10[0], n10[1], n10[2],
					1, 1,
					fx, y3, fz + 1,
					n01[0], n01[1], n01[2],
					0, 0,

					fx, y3, fz + 1,
					n01[0], n01[1], n01[2],
					0, 0,
					fx + 1, y2, fz,
					n10[0], n10[1], n10[2],
					1, 1,
					fx + 1, y4, fz + 1,
					n11[0], n11[1], n11[2],
					1, 0,
				}...)
			}
		}
	}
	return h
}

func loadTexture(device *d3d9.Device, path string) *d3d9.Texture {
	img := loadPng(path)
	texture, err := device.CreateTexture(
		uint(img.Bounds().Dx()),
		uint(img.Bounds().Dy()),
		1,
		d3d9.USAGE_SOFTWAREPROCESSING,
		d3d9.FMT_A8R8G8B8,
		d3d9.POOL_MANAGED,
		0,
	)
	check(err)
	r, err := texture.LockRect(0, nil, d3d9.LOCK_DISCARD)
	check(err)
	r.SetAllBytes(img.Pix, img.Stride)
	check(texture.UnlockRect(0))
	return texture
}

func loadPng(path string) *image.RGBA {
	f, err := open(path)
	check(err)
	defer f.Close()

	img, err := png.Decode(f)
	check(err)

	if n, ok := img.(*image.RGBA); ok {
		return n
	} else {
		n := image.NewRGBA(img.Bounds())
		draw.Draw(n, n.Bounds(), img, img.Bounds().Min, draw.Src)
		return n
	}
}

func open(path string) (io.ReadCloser, error) {
	data, err := payload.Open()
	if err != nil {
		return os.Open(path)
	}
	defer data.Close()
	dataBlob, err := blob.Read(data)
	if err != nil {
		return nil, err
	}
	d, found := dataBlob.GetByID(path)
	if !found {
		return nil, errors.New("data for '" + path + "' not found in payload blob")
	}
	return dummyCloser{bytes.NewReader(d)}, nil
}

type dummyCloser struct {
	io.Reader
}

func (dummyCloser) Close() error { return nil }

func createVertexBuffer(device *d3d9.Device, data []float32) *d3d9.VertexBuffer {
	buf, err := device.CreateVertexBuffer(
		uint(len(data))*4,
		d3d9.USAGE_WRITEONLY,
		0,
		d3d9.POOL_DEFAULT,
		0,
	)
	check(err)
	mem, err := buf.Lock(0, 0, d3d9.LOCK_DISCARD)
	check(err)
	mem.SetFloat32s(0, data)
	check(buf.Unlock())
	return buf
}

func rad2deg(x float32) float32 {
	return x * 180 / math.Pi
}

func deg2rad(x float32) float32 {
	return x * math.Pi / 180
}

func updateGame() {
	if gameState.keyJumpDown && !gameState.inAir {
		gameState.inAir = true
		gameState.velY = gameState.jumpSpeed
	}
	gameState.keyJumpDown = false

	if gameState.inAir {
		gameState.pos[1] += gameState.velY
		gameState.velY += gameState.gravity
	}

	mouseDx := gameState.mouseX - gameState.centerX
	mouseDy := gameState.mouseY - gameState.centerY
	w32.SetCursorPos(gameState.centerX, gameState.centerY)

	speed := gameState.moveSpeed
	if gameState.keyRunDown {
		speed *= runSpeedMultiplier
	} else if gameState.keySneakDown {
		speed *= sneakSpeedMultiplier
	}
	moveDir := gameState.viewDir
	moveDir[1] = 0
	moveDir = moveDir.Normalized()
	if gameState.keyForwardDown {
		gameState.pos = gameState.pos.Add(moveDir.MulScalar(speed))
	}
	if gameState.keyBackwardDown {
		gameState.pos = gameState.pos.Add(moveDir.MulScalar(-speed))
	}
	if gameState.keyLeftDown {
		gameState.pos = gameState.pos.Add(
			gameState.viewDir.Cross(d3dmath.Vec3{0, 1, 0}).MulScalar(speed),
		)
	}
	if gameState.keyRightDown {
		gameState.pos = gameState.pos.Add(
			d3dmath.Vec3{0, 1, 0}.Cross(gameState.viewDir).MulScalar(speed),
		)
	}
	if mouseDx != 0 {
		gameState.viewDir = gameState.viewDir.Homgeneous().MulMat(
			d3dmath.RotateY(deg2rad(float32(mouseDx) * 0.125)),
		).DropW().Normalized()
	}
	if mouseDy != 0 {
		gameState.viewDir[1] -= float32(mouseDy) / 500
		gameState.viewDir = gameState.viewDir.Normalized()
	}

	y := heightAt(gameState.pos[0], gameState.pos[2], ground)
	if gameState.pos[1] < y {
		gameState.inAir = false
	}
	if !gameState.inAir {
		gameState.pos[1] = y
	}

	if gameState.keyShootDown {
		gameState.keyShootDown = false
		origin := gameState.pos.Add(
			d3dmath.Vec3{0, gameState.playerHeight * 0.9, 0},
		)
		step := 1.0 * min(ground.scale[0], ground.scale[2]) * 0.5
		dir := gameState.viewDir.MulScalar(step)
		p := origin
		const maxShootSteps = 1000
		for i := 0; i < maxShootSteps; i++ {
			y := heightAt(p[0], p[2], ground)
			if p[1] <= y {
				// TODO at this point maybe iterate a couple of times with dir/2
				// towards the actual collision point; the closer it is to the
				// player, the more important it is to locate it properly since
				// the player will see it better
				p[1] = y
				break
			}
			p = p.Add(dir)
		}
		shootLaser(origin, p)
	}

	i := 0
	for i < len(gameState.laserBeams) {
		if gameState.laserBeams[i].life <= 0 {
			gameState.laserBeams = append(
				gameState.laserBeams[:i],
				gameState.laserBeams[i+1:]...,
			)
		} else {
			gameState.laserBeams[i].life += laserBeamDecay
			i++
		}
	}
}

func min(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func shootLaser(from, to d3dmath.Vec3) {
	gameState.laserBeams = append(gameState.laserBeams, laserBeam{
		life:  1,
		start: from,
		end:   to,
	})
}

type laserBeam struct {
	life       float32
	start, end d3dmath.Vec3
}

func skyMVP() d3dmath.Mat4 {
	m := d3dmath.Translate(0, 0, 0)
	v := d3dmath.LookAt(
		d3dmath.Vec3{},
		gameState.viewDir,
		d3dmath.Vec3{0, 1, 0},
	)
	p := d3dmath.Perspective(
		deg2rad(fieldOfViewDeg),
		float32(windowW)/float32(windowH),
		100,
		0.001,
	)
	return d3dmath.Mul4(m, v, p)
}

func renderGeometry(device *d3d9.Device) {
	camPos := gameState.pos
	camPos[1] += gameState.playerHeight
	v := d3dmath.LookAt(
		camPos,
		camPos.Add(gameState.viewDir),
		d3dmath.Vec3{0, 1, 0},
	)
	p := d3dmath.Perspective(
		deg2rad(fieldOfViewDeg),
		float32(windowW)/float32(windowH),
		100,
		0.001,
	)
	vp := d3dmath.Mul4(v, p)

	//caps, err := device.GetDeviceCaps()
	//check(err)
	//check(device.SetSamplerState(0, d3d9.SAMP_MAXANISOTROPY, caps.MaxAnisotropy))
	//check(device.SetSamplerState(0, d3d9.SAMP_MINFILTER, d3d9.TEXF_LINEAR))
	//check(device.SetSamplerState(0, d3d9.SAMP_MAGFILTER, d3d9.TEXF_LINEAR))
	//check(device.SetSamplerState(0, d3d9.SAMP_MIPFILTER, d3d9.TEXF_LINEAR))

	check(device.SetVertexShader(texVS))
	check(device.SetPixelShader(texPS))
	//check(device.SetVertexShaderConstantF(4, []float32{gameState.red, 0, 1, 1}))
	check(device.SetVertexDeclaration(texDecl))

	// draw sky box
	check(device.SetRenderState(d3d9.RS_ZENABLE, d3d9.ZB_FALSE))
	skyMVP := skyMVP().Transposed() // shader expected column-major ordering
	check(device.SetVertexShaderConstantF(0, skyMVP[:]))
	check(device.SetTexture(0, sky))
	check(device.SetStreamSource(0, skyVertices, 0, (3+2)*4))
	device.DrawPrimitive(d3d9.PT_TRIANGLELIST, 0, 12)

	// draw triangles
	check(device.SetRenderState(d3d9.RS_CULLMODE, d3d9.CULL_NONE))
	check(device.SetRenderState(d3d9.RS_ZENABLE, d3d9.ZB_TRUE))
	triMVP := vp.Transposed() // shader expected column-major ordering
	check(device.SetVertexShaderConstantF(0, triMVP[:]))
	check(device.SetTexture(0, texture))
	check(device.SetStreamSource(0, triangles, 0, (3+2)*4))
	device.DrawPrimitive(d3d9.PT_TRIANGLELIST, 0, 2)
	check(device.SetRenderState(d3d9.RS_CULLMODE, d3d9.CULL_CW))

	// draw floor
	check(device.SetVertexShader(texLitVS))
	check(device.SetPixelShader(texLitPS))
	check(device.SetVertexDeclaration(texLitDecl))
	size := ground.size()
	floorMVP := ground.modelTransform().Mul(vp).Transposed()
	check(device.SetVertexShaderConstantF(0, floorMVP[:]))
	check(device.SetTexture(0, floor))
	check(device.SetStreamSource(0, floorVertices, 0, (3+3+2)*4))
	device.DrawPrimitive(d3d9.PT_TRIANGLELIST, 0, uint(size*size*2))

	// draw laser beams
	if len(gameState.laserBeams) > 0 {
		check(device.SetRenderState(d3d9.RS_ALPHABLENDENABLE, 1))
		check(device.SetRenderState(d3d9.RS_SRCBLEND, d3d9.BLEND_SRCALPHA))
		check(device.SetRenderState(d3d9.RS_DESTBLEND, d3d9.BLEND_INVSRCALPHA))
		check(device.SetVertexDeclaration(uniColorDecl))
		check(device.SetVertexShader(uniColorVS))
		check(device.SetPixelShader(uniColorPS))
		check(device.SetStreamSource(0, square, 0, 3*4))
		for _, beam := range gameState.laserBeams {
			diff := beam.end.Sub(beam.start)
			length := diff.Norm()
			scale := d3dmath.Scale(0.005, 1, length)
			offset := d3dmath.TranslateV(beam.start)
			yRad := math.Atan2(float64(diff[2]), float64(diff[0]))
			rotY := d3dmath.RotateY(math.Pi/2 - float32(yRad))
			xRad := math.Atan2(float64(length), float64(-diff[1]))
			rotX := d3dmath.RotateX(math.Pi/2 - float32(xRad))
			m := d3dmath.Mul4(scale, rotX, rotY, offset)
			mvp := d3dmath.Mul4(m, vp).Transposed()
			check(device.SetPixelShaderConstantF(0, []float32{1, 0, 0, beam.life}))
			check(device.SetVertexShaderConstantF(0, mvp[:]))
			device.DrawPrimitive(d3d9.PT_TRIANGLELIST, 0, 2)
		}
		check(device.SetRenderState(d3d9.RS_ALPHABLENDENABLE, 0))
	}
}

// TODO bites me a lot: implicit connection between
// - CreateVertexDeclaration
// - SetStreamSource
// - createVertexBuffer
// these all have to know what a vertex for the shader is made of

const (
	fieldOfViewDeg       = 60
	runSpeedMultiplier   = 2
	sneakSpeedMultiplier = 0.5
	laserBeamDecay       = -0.05
)

var gameState struct {
	centerX, centerY int
	mouseX, mouseY   int

	keyForwardDown  bool
	keyBackwardDown bool
	keyLeftDown     bool
	keyRightDown    bool
	keyRunDown      bool
	keySneakDown    bool
	keyJumpDown     bool
	keyShootDown    bool

	moveSpeed    float32
	pos          d3dmath.Vec3 // player position in the world
	viewDir      d3dmath.Vec3 // must be kept unit length
	playerHeight float32
	velY         float32
	inAir        bool
	jumpSpeed    float32
	gravity      float32
	laserBeams   []laserBeam
}

func init() {
	gameState.moveSpeed = 0.03
	gameState.jumpSpeed = 0.046
	gameState.gravity = -0.0025
	gameState.playerHeight = 0.4
	gameState.pos = d3dmath.Vec3{0, 0, 0}
	gameState.viewDir = d3dmath.Vec3{0, 0, 1}.Normalized()
}
