package bmp

import (
	"errors"
	"strings"
	"testing"
)

var testPal = [][3]byte{
	{0, 0, 0},     // 0: black (default fill)
	{255, 0, 0},   // 1: red
	{0, 255, 0},   // 2: green
	{0, 0, 255},   // 3: blue
	{255, 255, 0}, // 4: yellow
}

func TestRLE8EncodedAndAbsoluteRuns(t *testing.T) {
	// 4x2 bottom-up.
	// Bottom row: run of 2x index1, then 1x index2, 1x index3.
	// Top row: absolute run of 3 bytes [4,1,2] (odd, padded), then 1x index3.
	stream := []byte{
		2, 1, // bottom: 1,1
		1, 2, 1, 3, // 2, 3
		0, 0, // EOL
		0, 3, 4, 1, 2, 0, // absolute 3 + pad
		1, 3, // 3
		0, 1, // end of bitmap
	}
	img, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		x, y int
		want [3]byte
	}{
		{0, 0, testPal[4]}, {1, 0, testPal[1]}, {2, 0, testPal[2]}, {3, 0, testPal[3]},
		{0, 1, testPal[1]}, {1, 1, testPal[1]}, {2, 1, testPal[2]}, {3, 1, testPal[3]},
	}
	for _, c := range checks {
		r, g, b := pixel(t, img, c.x, c.y)
		if [3]byte{r, g, b} != c.want {
			t.Fatalf("(%d,%d) = %v, want %v", c.x, c.y, [3]byte{r, g, b}, c.want)
		}
	}
}

func TestRLE8DeltaAndDefaultFill(t *testing.T) {
	// 4x3. Delta (1,1) then one pixel of index 2; rest stays index 0.
	stream := []byte{
		0, 2, 1, 1, // delta right 1, up 1
		1, 2, // one pixel index 2 at (1,1)
		0, 1, // end
	}
	img, err := Decode(buildBMP(4, 3, 8, biRLE8, testPal, stream))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b := pixel(t, img, 1, 1); r != 0 || g != 255 || b != 0 {
		t.Fatalf("(1,1) = %d,%d,%d", r, g, b)
	}
	if r, g, b := pixel(t, img, 0, 0); r != 0 || g != 0 || b != 0 {
		t.Fatalf("(0,0) = %d,%d,%d, want default black", r, g, b)
	}
	if r, g, b := pixel(t, img, 3, 2); r != 0 || g != 0 || b != 0 {
		t.Fatalf("(3,2) = %d,%d,%d, want default black", r, g, b)
	}
}

func TestRLE8RejectsNegativeHeight(t *testing.T) {
	stream := []byte{0, 1}
	_, err := Decode(buildBMP(2, -2, 8, biRLE8, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("want positive-height rejection, got %v", err)
	}
}

func TestRLE8RunCrossingRowWidth(t *testing.T) {
	stream := []byte{5, 1, 0, 1} // run of 5 in width-4 image
	_, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	var oe *OffsetError
	if !errors.As(err, &oe) || !strings.Contains(err.Error(), "crosses row width") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8DeltaOutOfBounds(t *testing.T) {
	stream := []byte{0, 2, 10, 0, 0, 1}
	_, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "delta") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8MissingEndMarker(t *testing.T) {
	stream := []byte{2, 1} // clean command boundary, no end marker
	_, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "without end-of-bitmap marker") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8Truncated(t *testing.T) {
	stream := []byte{3} // single dangling byte
	_, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8TruncatedAbsoluteRun(t *testing.T) {
	stream := []byte{0, 4, 1, 2} // absolute 4 but only 2 bytes present
	_, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "truncated absolute run") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8BadPaletteIndex(t *testing.T) {
	stream := []byte{1, 9, 0, 1} // index 9, palette has 5
	_, err := Decode(buildBMP(4, 2, 8, biRLE8, testPal, stream))
	var oe *OffsetError
	if !errors.As(err, &oe) || !strings.Contains(err.Error(), "palette index 9") {
		t.Fatalf("got %v", err)
	}
}
