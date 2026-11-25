package recording

import "testing"

func TestViewImageAndVisible(t *testing.T) {
	// A 16:9 screen in a 16:9 frame: the image is the frame.
	v := View{FrameW: 1920, FrameH: 1080, SrcW: 2560, SrcH: 1440, Fit: "cover"}

	if w, h := v.Image(); w != 1920000 || h != 1080000 {
		t.Errorf("image %d x %d", w, h)
	}

	if vw, vh := v.Visible(); vw != Unit || vh != Unit {
		t.Errorf("visible %d x %d", vw, vh)
	}

	// The same screen in a vertical frame: cover crops the sides.
	v = View{FrameW: 1080, FrameH: 1920, SrcW: 1920, SrcH: 1080, Fit: "cover"}

	if w, h := v.Image(); w != 3413333 || h != 1920000 {
		t.Errorf("vertical image %d x %d", w, h)
	}

	if vw, vh := v.Visible(); vw != 3164 || vh != Unit {
		t.Errorf("vertical visible %d x %d", vw, vh)
	}

	// Contain keeps the whole screen: a band of the frame stays empty.
	v.Fit = "contain"

	if w, h := v.Image(); w != 1080000 || h != 607500 {
		t.Errorf("contain image %d x %d", w, h)
	}
}

func TestViewCentreKeepsThePictureOverTheFrame(t *testing.T) {
	v := View{FrameW: 1920, FrameH: 1080, SrcW: 1920, SrcH: 1080, Fit: "cover"}

	// Focus at the centre: the item stays centred whatever the zoom.
	if x, y := v.Centre(5000, 5000, 1600); x != 960000 || y != 540000 {
		t.Errorf("centre focus %d, %d", x, y)
	}

	// Focus on the top left corner at 2x: the picture is held against the corner, not past it.
	x, y := v.Centre(0, 0, 2000)
	if x != 1920000 || y != 1080000 {
		t.Errorf("corner focus %d, %d, want the picture's left edge at the frame's left edge (1920000, 1080000)", x, y)
	}

	// A focus a quarter in at 2x puts that point at the centre of the frame.
	x, _ = v.Centre(2500, 5000, 2000)
	if px, _ := v.Project(480, 540, 2000, x, 540000); px < 959000 || px > 961000 {
		t.Errorf("the focus lands at %d, not at the centre", px)
	}
}

func TestViewCentreOnAnAxisTheFrameFills(t *testing.T) {
	// Contain: the image is shorter than the frame, so the item stays centred on that axis.
	v := View{FrameW: 1080, FrameH: 1920, SrcW: 1920, SrcH: 1080, Fit: "contain"}

	if _, y := v.Centre(5000, 2000, 1000); y != 960000 {
		t.Errorf("y %d", y)
	}
}

func TestViewProject(t *testing.T) {
	v := View{FrameW: 1920, FrameH: 1080, SrcW: 1920, SrcH: 1080, Fit: "cover"}

	// Unzoomed and centred, the pixel (0, 0) is half a pixel in from the top left corner.
	if x, y := v.Project(0, 0, 1000, 960000, 540000); x != 500 || y != 500 {
		t.Errorf("%d, %d", x, y)
	}

	if x, y := v.Project(1919, 1079, 1000, 960000, 540000); x != 1919500 || y != 1079500 {
		t.Errorf("%d, %d", x, y)
	}
}
