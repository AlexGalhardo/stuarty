package timeval

import (
	"errors"
	"strings"
	"testing"
)

var rates = []FPS{{24, 1}, {25, 1}, {30, 1}, {60, 1}, {30000, 1001}}

func mustParse(t *testing.T, s string, fps FPS) int64 {
	t.Helper()
	f, err := Parse(s, fps)
	if err != nil {
		t.Fatalf("Parse(%q, %v): %v", s, fps, err)
	}
	return f
}

func TestParseTable(t *testing.T) {
	cases := []struct {
		in   string
		fps  FPS
		want int64
	}{
		{"12.5s", FPS{30, 1}, 375},
		{"375f", FPS{30, 1}, 375},
		{"00:00:12.500", FPS{30, 1}, 375},
		{"0s", FPS{30, 1}, 0},
		{"0f", FPS{24, 1}, 0},
		{"1s", FPS{24, 1}, 24},
		{"1s", FPS{25, 1}, 25},
		{"1s", FPS{60, 1}, 60},
		{"12.5s", FPS{24, 1}, 300},
		{"12.5s", FPS{25, 1}, 313}, // 312.5 rounds up
		{"0.04s", FPS{25, 1}, 1},
		{"0.02s", FPS{25, 1}, 1}, // 0.5 frame rounds up
		{"0.019s", FPS{25, 1}, 0},
		{"12.48s", FPS{30, 1}, 374}, // SDD-002 A.4 example: 374.4
		{"1001s", FPS{30000, 1001}, 30000},
		{"1s", FPS{30000, 1001}, 30}, // 29.97 rounds to 30
		{"10s", FPS{30000, 1001}, 300},
		{"01:02.5", FPS{30, 1}, 1875},
		{"01:00:00", FPS{30, 1}, 108000},
		{"00:00:00.000", FPS{60, 1}, 0},
		{"1:1:1", FPS{1, 1}, 3661},
	}
	for _, c := range cases {
		if got := mustParse(t, c.in, c.fps); got != c.want {
			t.Errorf("Parse(%q, %v) = %d, want %d", c.in, c.fps, got, c.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	bad := []string{
		"",
		"12",
		"12.5",
		"-12s",
		"-375f",
		"-00:00:01",
		"+12s",
		"12ms",
		"12m",
		"12S",
		"12.5f",
		"375.0f",
		"f",
		"s",
		".5s",
		"12.s",
		"1e3s",
		" 12s",
		"12s ",
		"12 s",
		"0x10f",
		"abc",
		"1:2:3:4",
		":",
		"00:",
		":10",
		"00:60",
		"00:00:60",
		"01:60:00",
		"00:00:1x",
		"a:00:01",
		"00:00:-1",
		"00:00:01.",
		"00:00:.5",
		"1234567890:00:00",
		"99999999999999999999f",
		"99999999999999999999s",
		"1099511627777f",
		"100000000000s",
		"000000000000000000000000000000000000000000000000000000000000000001s",
		"1.5.5s",
		"0:0:0.0.0",
	}
	for _, s := range bad {
		_, err := Parse(s, FPS{30, 1})
		var te *Error
		if !errors.As(err, &te) || te.Code != "E_INVALID_TIME" || te.Hint == "" {
			t.Errorf("Parse(%q) error = %v, want E_INVALID_TIME with hint", s, err)
		}
	}
}

func TestParseBareNumberMessage(t *testing.T) {
	_, err := Parse("12", FPS{30, 1})
	if err == nil || !strings.Contains(err.Error(), "E_INVALID_TIME") || !strings.Contains(err.Error(), "no unit") {
		t.Fatalf("got %v", err)
	}
}

func TestParseInvalidFPS(t *testing.T) {
	for _, f := range []FPS{{}, {0, 1}, {30, 0}, {-30, 1}, {1, 2}, {2000, 1}, {2_000_000, 1_000_000}} {
		if _, err := Parse("1s", f); !errors.Is(err, ErrInvalidFPS) {
			t.Errorf("Parse with %v: %v", f, err)
		}
		if _, err := NewFPS(f.Num, f.Den); !errors.Is(err, ErrInvalidFPS) {
			t.Errorf("NewFPS(%v): %v", f, err)
		}
	}
	if f, err := NewFPS(30000, 1001); err != nil || f != (FPS{30000, 1001}) {
		t.Fatalf("NewFPS: %v %v", f, err)
	}
}

func TestFPSString(t *testing.T) {
	if s := (FPS{30, 1}).String(); s != "30" {
		t.Error(s)
	}
	if s := (FPS{30000, 1001}).String(); s != "30000/1001" {
		t.Error(s)
	}
}

func TestRoundingHalfAwayFromZero(t *testing.T) {
	f := FPS{25, 1}
	if got := f.FrameAtMillis(20); got != 1 { // 0.5 frame
		t.Errorf("20ms = %d", got)
	}
	if got := f.FrameAtMillis(-20); got != -1 {
		t.Errorf("-20ms = %d", got)
	}
	if got := f.FrameAtMillis(19); got != 0 {
		t.Errorf("19ms = %d", got)
	}
	if got := f.Millis(-1); got != -40 {
		t.Errorf("Millis(-1) = %d", got)
	}
	if got := (FPS{30000, 1001}).Millis(1); got != 33 {
		t.Errorf("Millis(1) at 29.97 = %d", got)
	}
}

func TestFormat(t *testing.T) {
	f := FPS{30, 1}
	if s := f.Timecode(375); s != "00:00:12.500" {
		t.Error(s)
	}
	if s := f.Timecode(108000); s != "01:00:00.000" {
		t.Error(s)
	}
	if s := f.Timecode(-3); s != "-00:00:00.100" {
		t.Error(s)
	}
	if s := f.FormatSeconds(375); s != "12.5s" {
		t.Error(s)
	}
	if s := f.FormatSeconds(60); s != "2s" {
		t.Error(s)
	}
	if s := f.FormatSeconds(0); s != "0s" {
		t.Error(s)
	}
	if s := f.FormatSeconds(-45); s != "-1.5s" {
		t.Error(s)
	}
	if s := f.FormatSeconds(374); s != "12.467s" {
		t.Error(s)
	}
	if s := FormatFrames(375); s != "375f" {
		t.Error(s)
	}
}

func TestEffective(t *testing.T) {
	e := FPS{30, 1}.Effective(374)
	if e.Frame != 374 || e.Seconds != 12.467 || e.Timecode != "00:00:12.467" || e.Requested != "" {
		t.Errorf("%+v", e)
	}
}

func TestRoundTripEveryFrame(t *testing.T) {
	for _, fps := range rates {
		for frame := range int64(108001) {
			forms := []string{FormatFrames(frame), fps.FormatSeconds(frame), fps.Timecode(frame)}
			for _, s := range forms {
				got, err := Parse(s, fps)
				if err != nil || got != frame {
					t.Fatalf("fps %v frame %d: %q parsed to %d, %v", fps, frame, s, got, err)
				}
			}
		}
	}
}

func TestMaxFrameBoundary(t *testing.T) {
	if got := mustParse(t, "1099511627776f", FPS{30, 1}); got != MaxFrame {
		t.Error(got)
	}
	f := FPS{1000, 1}
	if got := f.FrameAtMillis(f.Millis(MaxFrame)); got != MaxFrame {
		t.Error(got)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"12.5s", "375f", "00:00:12.500", "01:02.5", "12", "-1s", "", "1:2:3", "1.s", "99999999999999999999s", "0f"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, fps := range rates {
			frame, err := Parse(s, fps)
			if err != nil {
				var te *Error
				if !errors.As(err, &te) || te.Code != CodeInvalidTime {
					t.Fatalf("Parse(%q): unexpected error %v", s, err)
				}
				continue
			}
			if frame < 0 || frame > MaxFrame {
				t.Fatalf("Parse(%q) = %d out of range", s, frame)
			}
			// What Parse accepts, Format must be able to write and Parse to read back.
			if back, err := Parse(FormatFrames(frame), fps); err != nil || back != frame {
				t.Fatalf("frames round trip of %q: %d, %v", s, back, err)
			}
			if back, err := Parse(fps.Timecode(frame), fps); err != nil || back != frame {
				t.Fatalf("timecode round trip of %q: %d, %v", s, back, err)
			}
		}
	})
}
