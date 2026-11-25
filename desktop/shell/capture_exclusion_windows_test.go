//go:build windows

package shell

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The teleprompter window is kept out of a capture with SetWindowDisplayAffinity (SDD-025 section 20.4, epic 66
// task 4). This test proves what the system does with that flag, with a real window and a real capture of the
// screen: a window of one flat colour is shown on the desktop, the test captures the screen where it is and counts
// the pixels of that colour, with the flag off (the window must be seen, so the test can fail), with the flag on
// (it must not be seen) and with the flag off again (it is seen again, so the flag is the cause).
//
// It needs a Windows desktop session and shows a small window for a few seconds, so it runs only when
// STUARTY_TEST_REAL_SCREEN is 1. Wails sets the same flag from ContentProtectionEnabled; that the window of the
// shipped application keeps it is the manual check of the epic (QUESTIONS.md).

const (
	wdaNone               = 0x0
	wdaExcludeFromCapture = 0x11

	wsPopup        = 0x80000000
	wsVisible      = 0x10000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000

	pmRemove   = 0x0001
	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000
	biRGB      = 0
	dibRGB     = 0
	swShowNA   = 8

	probeW = 240
	probeH = 160
	probeX = 80
	probeY = 80
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procUnregisterClass   = user32.NewProc("UnregisterClassW")
	procPeekMessage       = user32.NewProc("PeekMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procSetAffinity       = user32.NewProc("SetWindowDisplayAffinity")
	procGetAffinity       = user32.NewProc("GetWindowDisplayAffinity")
	procShowWindow        = user32.NewProc("ShowWindow")
	procSetProcessDPIAwar = user32.NewProc("SetProcessDPIAware")
	procGetDC             = user32.NewProc("GetDC")
	procReleaseDC         = user32.NewProc("ReleaseDC")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procCreateCompatDC   = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procBitBlt           = gdi32.NewProc("BitBlt")
	procGetDIBits        = gdi32.NewProc("GetDIBits")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procDeleteDC         = gdi32.NewProc("DeleteDC")
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSmall  uintptr
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

// probeWindow is a window of one flat colour, shown on top of the others without taking the focus.
type probeWindow struct {
	hwnd  uintptr
	class *uint16
}

// pump runs the messages of the thread that made the window until the deadline passes.
func pump(wait time.Duration) {
	var m winMsg

	deadline := time.Now().Add(wait)

	for time.Now().Before(deadline) {
		for {
			got, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if got == 0 {
				break
			}

			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}

		<-time.After(10 * time.Millisecond)
	}
}

// newProbe shows a window of the flat colour (a COLORREF, 0x00BBGGRR) at a fixed place of the primary display.
func newProbe(t *testing.T, colour uint32) *probeWindow {
	t.Helper()

	brush, _, _ := procCreateSolidBrush.Call(uintptr(colour))
	proc := syscall.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)

		return r
	})

	name, err := syscall.UTF16PtrFromString("StuartyTeleprompterProbe")
	if err != nil {
		t.Fatal(err)
	}

	class := wndClassEx{wndProc: proc, background: brush, className: name}
	class.size = uint32(unsafe.Sizeof(class))

	if atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		t.Fatalf("RegisterClassEx: %v", callErr)
	}

	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExNoActivate, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)),
		wsPopup|wsVisible, probeX, probeY, probeW, probeH, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowEx: %v", callErr)
	}

	procShowWindow.Call(hwnd, swShowNA)

	return &probeWindow{hwnd: hwnd, class: name}
}

func (p *probeWindow) close() {
	procDestroyWindow.Call(p.hwnd)
	procUnregisterClass.Call(uintptr(unsafe.Pointer(p.class)), 0)
}

func (p *probeWindow) setAffinity(t *testing.T, affinity uintptr) {
	t.Helper()

	if ok, _, err := procSetAffinity.Call(p.hwnd, affinity); ok == 0 {
		t.Fatalf("SetWindowDisplayAffinity(%#x): %v", affinity, err)
	}

	var got uint32
	if ok, _, err := procGetAffinity.Call(p.hwnd, uintptr(unsafe.Pointer(&got))); ok == 0 || uintptr(got) != affinity {
		t.Fatalf("GetWindowDisplayAffinity = %#x, %v; want %#x", got, err, affinity)
	}
}

// captureProbeArea copies the area of the primary display where the window is, as the screen shows it, and counts the
// pixels that have the colour of the window.
func captureProbeArea(t *testing.T, colour uint32) int {
	t.Helper()

	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		t.Fatal("GetDC(screen) gave no device context: no desktop session")
	}

	defer procReleaseDC.Call(0, screen)

	mem, _, _ := procCreateCompatDC.Call(screen)
	defer procDeleteDC.Call(mem)

	bmp, _, _ := procCreateCompatBmp.Call(screen, probeW, probeH)
	defer procDeleteObject.Call(bmp)

	procSelectObject.Call(mem, bmp)

	if ok, _, err := procBitBlt.Call(mem, 0, 0, probeW, probeH, screen, probeX, probeY, srcCopy|captureBlt); ok == 0 {
		t.Fatalf("BitBlt: %v", err)
	}

	header := bitmapInfoHeader{width: probeW, height: -probeH, planes: 1, bitCount: 32, compression: biRGB}
	header.size = uint32(unsafe.Sizeof(header))
	pixels := make([]byte, probeW*probeH*4)

	if rows, _, err := procGetDIBits.Call(mem, bmp, 0, probeH, uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&header)), dibRGB); rows == 0 {
		t.Fatalf("GetDIBits: %v", err)
	}

	// The pixels are B, G, R, unused; the colour is 0x00BBGGRR.
	b, g, r := byte(colour>>16), byte(colour>>8), byte(colour)
	n := 0

	for i := 0; i+3 < len(pixels); i += 4 {
		if pixels[i] == b && pixels[i+1] == g && pixels[i+2] == r {
			n++
		}
	}

	return n
}

// waitFor pumps the window's messages and captures until want says the count is what it should be, or the time is
// up; it returns the last count. The compositor needs a moment to draw a window or to stop drawing it.
func waitFor(t *testing.T, colour uint32, want func(n int) bool) int {
	t.Helper()

	var n int

	deadline := time.Now().Add(8 * time.Second)

	for time.Now().Before(deadline) {
		pump(100 * time.Millisecond)

		if n = captureProbeArea(t, colour); want(n) {
			return n
		}
	}

	return n
}

func TestTeleprompterAffinityKeepsAWindowOutOfACapture(t *testing.T) {
	if os.Getenv("STUARTY_TEST_REAL_SCREEN") != "1" {
		t.Skip(
			"set STUARTY_TEST_REAL_SCREEN=1 on a Windows desktop: it shows a small window for a few seconds",
		) // verify:env-skip needs a desktop session and shows a window
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procSetProcessDPIAwar.Call()

	// A colour no desktop uses: magenta, as a COLORREF (0x00BBGGRR).
	const magenta = 0x00FF00FF

	probe := newProbe(t, magenta)
	defer probe.close()

	total := probeW * probeH
	seen := func(n int) bool { return n > total*9/10 }
	hidden := func(n int) bool { return n == 0 }

	if n := waitFor(t, magenta, seen); !seen(n) {
		t.Fatalf(
			"with the flag off the capture holds %d of %d pixels of the window: the test cannot see a window at all",
			n,
			total,
		)
	}

	probe.setAffinity(t, wdaExcludeFromCapture)

	if n := waitFor(t, magenta, hidden); !hidden(n) {
		t.Errorf("with WDA_EXCLUDEFROMCAPTURE the capture still holds %d pixels of the window", n)
	}

	probe.setAffinity(t, wdaNone)

	if n := waitFor(t, magenta, seen); !seen(n) {
		t.Errorf(
			"with the flag off again the capture holds %d of %d pixels: the flag was not what hid the window",
			n,
			total,
		)
	}
}

// The same claim with the real helper of the product (SDD-025 section 20.4): the helper records the first display with
// Windows Graphics Capture while the window is up, once with the flag off (the window must be in the last frame, so
// the test can fail) and once with it on (none of its pixels may be). The helper is the one of `capture/`, built by
// the person who runs the test, and a test build of it (--allow-test-source) writes the frames raw, so no encoder
// stands between the display and the pixels that are counted.
//
// It needs STUARTY_TEST_REAL_SCREEN=1, STUARTY_TEST_CAPTURE_HELPER (the stuarty-capture.exe) and
// STUARTY_TEST_FFMPEG_DIR (the helper asks for an FFmpeg path even when it writes raw frames), and the window must be
// on the first display the helper lists.

type helperLine struct {
	Type     string `json:"type"`
	Raw      string `json:"-"`
	Reason   string `json:"reason"`
	Frames   int    `json:"frames"`
	Displays []struct {
		ID     string `json:"id"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"displays"`
}

// recordFirstDisplay records the first display for a moment into a raw file and returns its size and the last frame.
func recordFirstDisplay(t *testing.T, helper, ffmpeg, dir string) (width, height int, last []byte) {
	t.Helper()

	out := filepath.Join(dir, "capture.raw")
	cmd := exec.Command(helper, "--allow-test-source")
	cmd.Stderr = nil

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("the helper did not start: %v", err)
	}

	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	lines := make(chan helperLine, 64)

	go func() {
		defer close(lines)

		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 1<<20), 1<<20)

		for scan.Scan() {
			var l helperLine
			if json.Unmarshal(scan.Bytes(), &l) == nil {
				l.Raw = scan.Text()
				lines <- l
			}
		}
	}()

	next := func(want string) helperLine {
		deadline := time.After(20 * time.Second)

		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("the helper closed its output before %q", want)
				}

				if l.Type == want {
					return l
				}

				if l.Type == "error" {
					t.Fatalf("the helper answered an error while waiting for %q: %s", want, l.Raw)
				}
			case <-deadline:
				t.Fatalf("the helper did not send %q in time", want)
			}
		}
	}

	send := func(line string) {
		if _, err := stdin.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
	}

	next("ready")
	send(`{"type":"sources"}`)

	sources := next("sources")
	if len(sources.Displays) == 0 {
		t.Fatal("the helper lists no display")
	}

	first := sources.Displays[0]
	width, height = first.Width, first.Height

	esc := filepath.ToSlash // the helper reads forward slashes as well, and its JSON needs no escape then
	send(
		fmt.Sprintf(
			`{"type":"start","recording":"rec_probe","source":{"kind":"display","id":%q},"fps":24,"encoder":"raw",`+
				`"ffmpeg":"%s","output":"%s"}`,
			first.ID,
			esc(ffmpeg),
			esc(out),
		),
	)
	next("started")
	pump(700 * time.Millisecond)
	send(`{"type":"stop"}`)

	if fin := next("finished"); fin.Frames < 2 {
		t.Fatalf("the helper wrote %d frames", fin.Frames)
	}

	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	size := int64(width * height * 4)

	info, err := file.Stat()
	if err != nil || info.Size() < size {
		t.Fatalf("the raw file holds %d bytes, less than a frame of %d: %v", info.Size(), size, err)
	}

	last = make([]byte, size)
	if _, err := file.ReadAt(last, (info.Size()/size-1)*size); err != nil {
		t.Fatal(err)
	}

	return width, height, last
}

// magentaIn counts the pixels of the colour in the probe's rectangle of a raw BGRA frame.
func magentaIn(frame []byte, width, height int) int {
	n := 0

	for y := probeY; y < min(probeY+probeH, height); y++ {
		for x := probeX; x < min(probeX+probeW, width); x++ {
			i := (y*width + x) * 4
			if frame[i] == 0xFF && frame[i+1] == 0x00 && frame[i+2] == 0xFF {
				n++
			}
		}
	}

	return n
}

func TestTeleprompterAffinityKeepsAWindowOutOfTheHelperCapture(t *testing.T) {
	helper, ffmpegDir := os.Getenv("STUARTY_TEST_CAPTURE_HELPER"), os.Getenv("STUARTY_TEST_FFMPEG_DIR")
	if os.Getenv("STUARTY_TEST_REAL_SCREEN") != "1" || helper == "" || ffmpegDir == "" {
		t.Skip(
			"set STUARTY_TEST_REAL_SCREEN=1, STUARTY_TEST_CAPTURE_HELPER and STUARTY_TEST_FFMPEG_DIR on a Windows desktop",
		) // verify:env-skip needs a desktop session, the built helper and the test FFmpeg
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procSetProcessDPIAwar.Call()

	const magenta = 0x00FF00FF

	probe := newProbe(t, magenta)
	defer probe.close()

	pump(500 * time.Millisecond)

	ffmpeg := filepath.Join(ffmpegDir, "ffmpeg.exe")
	total := probeW * probeH

	width, height, frame := recordFirstDisplay(t, helper, ffmpeg, t.TempDir())
	if n := magentaIn(frame, width, height); n < total*9/10 {
		t.Fatalf(
			"with the flag off the helper's last frame holds %d of %d pixels of the window (display %dx%d): the window is not on the first display the helper lists, or the helper does not see it",
			n,
			total,
			width,
			height,
		)
	}

	probe.setAffinity(t, wdaExcludeFromCapture)
	pump(500 * time.Millisecond)

	width, height, frame = recordFirstDisplay(t, helper, ffmpeg, t.TempDir())
	if n := magentaIn(frame, width, height); n != 0 {
		t.Errorf("with WDA_EXCLUDEFROMCAPTURE the helper's last frame still holds %d pixels of the window", n)
	}
}
