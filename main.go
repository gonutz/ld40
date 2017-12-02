package main

import (
	"fmt"
	"github.com/gonutz/d3d9"
	"github.com/gonutz/d3dmath"
	"github.com/gonutz/w32"
	"github.com/gonutz/win"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

var windowW, windowH = 640, 480

func main() {
	defer handlePanics()

	runtime.LockOSThread()

	win.HideConsoleWindow()

	var oldWindowPos w32.WINDOWPLACEMENT
	toggleFullscreen := func(window w32.HWND) {
		if win.IsFullscreen(window) {
			win.DisableFullscreen(window, oldWindowPos)
		} else {
			oldWindowPos = win.EnableFullscreen(window)
		}
	}

	window, err := win.NewWindow(
		w32.CW_USEDEFAULT,
		w32.CW_USEDEFAULT,
		windowW,
		windowH,
		"LD40window",
		func(window w32.HWND, msg uint32, w, l uintptr) uintptr {
			switch msg {
			case w32.WM_KEYDOWN:
				switch w {
				case w32.VK_UP:
					gameState.keyForwardDown = true
				case w32.VK_DOWN:
					gameState.keyBackwardDown = true
				case w32.VK_LEFT:
					gameState.keyLeftDown = true
				case w32.VK_RIGHT:
					gameState.keyRightDown = true
				case w32.VK_ESCAPE:
					win.CloseWindow(window)
				case w32.VK_F11:
					toggleFullscreen(window)
				}
				return 0
			case w32.WM_KEYUP:
				switch w {
				case w32.VK_UP:
					gameState.keyForwardDown = false
				case w32.VK_DOWN:
					gameState.keyBackwardDown = false
				case w32.VK_LEFT:
					gameState.keyLeftDown = false
				case w32.VK_RIGHT:
					gameState.keyRightDown = false
				}
				return 0
			case w32.WM_SIZE:
				windowW = int((uint(l)) & 0xFFFF)
				windowH = int((uint(l) >> 16) & 0xFFFF)
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
	toggleFullscreen(window)

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
		Windowed:         1,
		HDeviceWindow:    d3d9.HWND(window),
		SwapEffect:       d3d9.SWAPEFFECT_COPY, // so Present can use rects
		BackBufferWidth:  uint32(windowW),
		BackBufferHeight: uint32(windowH),
		BackBufferFormat: d3d9.FMT_UNKNOWN,
		BackBufferCount:  1,
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
		device.SetRenderState(d3d9.RS_CULLMODE, d3d9.CULL_NONE)
		device.SetRenderState(d3d9.RS_SRCBLEND, d3d9.BLEND_SRCALPHA)
		device.SetRenderState(d3d9.RS_DESTBLEND, d3d9.BLEND_INVSRCALPHA)
		device.SetRenderState(d3d9.RS_ALPHABLENDENABLE, 1)
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

			updateGame()

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
				device.SetViewport(
					d3d9.VIEWPORT{
						X:      0,
						Y:      0,
						Width:  uint32(windowW),
						Height: uint32(windowH),
						MinZ:   0,
						MaxZ:   1,
					},
				)

				device.Clear(
					nil,
					d3d9.CLEAR_TARGET,
					d3d9.ColorRGB(255, 0, 0),
					1,
					0,
				)
				device.BeginScene()
				renderGeometry(device)
				device.EndScene()
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
			"ide_log_"+time.Now().Format("2006_01_02__15_04_05")+".txt",
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
	colorVS   *d3d9.VertexShader
	colorPS   *d3d9.PixelShader
	colorDecl *d3d9.VertexDeclaration
	vertices  *d3d9.VertexBuffer

	// game related rendering data
	mvp d3dmath.Mat4
)

func createGeometry(device *d3d9.Device) {
	var err error

	colorVS, err = device.CreateVertexShaderFromBytes(vertexShader_uniform_color)
	check(err)
	colorPS, err = device.CreatePixelShaderFromBytes(pixelShader_uniform_color)
	check(err)

	colorDecl, err = device.CreateVertexDeclaration(
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
				Type:       d3d9.DECLTYPE_FLOAT4,
				Method:     d3d9.DECLMETHOD_DEFAULT,
				Usage:      d3d9.DECLUSAGE_COLOR,
				UsageIndex: 0,
			},
			d3d9.DeclEnd(),
		},
	)
	check(err)

	vertices = createVertexBuffer(device, []float32{
		0, 0, 0,
		1, 0, 0,
		0, 1, 0,
	})
}

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

func destroyGeometry() {
	if colorVS != nil {
		colorVS.Release()
		colorVS = nil
	}
	if colorPS != nil {
		colorPS.Release()
		colorPS = nil
	}
	if colorDecl != nil {
		colorDecl.Release()
		colorDecl = nil
	}
	if vertices != nil {
		vertices.Release()
		vertices = nil
	}
}

func rad2deg(x float32) float32 {
	return x * 180 / math.Pi
}

func deg2rad(x float32) float32 {
	return x * math.Pi / 180
}

func updateGame() {
	if gameState.keyForwardDown {
		gameState.camPos = gameState.camPos.Add(
			d3dmath.Vec3{0, 0, -gameState.moveSpeed},
		)
	}
	if gameState.keyBackwardDown {
		gameState.camPos = gameState.camPos.Add(
			d3dmath.Vec3{0, 0, gameState.moveSpeed},
		)
	}
	if gameState.keyLeftDown {
		gameState.camPos = gameState.camPos.Add(
			d3dmath.Vec3{gameState.moveSpeed, 0, 0},
		)
	}
	if gameState.keyRightDown {
		gameState.camPos = gameState.camPos.Add(
			d3dmath.Vec3{-gameState.moveSpeed, 0, 0},
		)
	}

	gameState.red += 0.01
	if gameState.red > 1 {
		gameState.red -= 1
	}

	gameState.rotDeg += 1.4

	m := d3dmath.RotateZ(deg2rad(gameState.rotDeg))
	m = d3dmath.Mul4(m, d3dmath.RotateX(deg2rad(gameState.rotDeg*0.753)))
	m = d3dmath.Mul4(m, d3dmath.RotateY(deg2rad(gameState.rotDeg*1.174)))
	v := d3dmath.Translate(
		gameState.camPos[0],
		gameState.camPos[1],
		gameState.camPos[2],
	)
	p := d3dmath.Perspective(
		deg2rad(fieldOfViewDeg),
		float32(windowW)/float32(windowH),
		0.001,
		100,
	)
	mvp = d3dmath.Mul4(m, v, p).Transposed()
}

func renderGeometry(device *d3d9.Device) {
	check(device.SetVertexShader(colorVS))
	check(device.SetPixelShader(colorPS))
	check(device.SetVertexShaderConstantF(0, mvp[:]))
	check(device.SetVertexShaderConstantF(4, []float32{gameState.red, 0, 1, 1}))
	check(device.SetVertexDeclaration(colorDecl))
	check(device.SetStreamSource(0, vertices, 0, 3*4))
	device.DrawPrimitive(d3d9.PT_TRIANGLELIST, 0, 1)
}

const fieldOfViewDeg = 90

var gameState struct {
	keyForwardDown  bool
	keyBackwardDown bool
	keyLeftDown     bool
	keyRightDown    bool

	rotDeg    float32
	red       float32
	moveSpeed float32
	camPos    d3dmath.Vec3
	viewDir   d3dmath.Vec3
}

func init() {
	gameState.moveSpeed = 0.1
	gameState.camPos = d3dmath.Vec3{0, 0, 2}
	gameState.viewDir = d3dmath.Vec3{0, 0, 0}
}
