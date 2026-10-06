package bmpintake

import (
	"strings"
	"testing"
)

var testPal = []RGBQuad{
	{R: 0, G: 0, B: 0},
	{R: 255, G: 255, B: 255},
	{R: 255, G: 0, B: 0},
}

func TestRLE8Basic(t *testing.T) {
	// 4x2, bottom-up. Stream order: bottom row first.
	stream := []byte{
		3, 1, // bottom: three 1s
		0, 0, // EOL
		0, 3, 2, 2, 2, 0, // top: absolute run of three 2s (odd, padded)
		0, 1, // EOB
	}
	img, err := Decode(buildBMP(4, 2, 8, 1, testPal, stream))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		2, 2, 2, 0, // top row
		1, 1, 1, 0, // bottom row; unwritten x=3 stays palette index 0
	}
	for i, v := range want {
		if img.Pixels[i] != v {
			t.Fatalf("pixel %d = %d, want %d (all %v)", i, img.Pixels[i], v, img.Pixels)
		}
	}
}

func TestRLE8Delta(t *testing.T) {
	// 4x3. Skip (1,1) from origin then write two 1s, then EOB.
	stream := []byte{
		0, 2, 1, 1, // delta dx=1 dy=1
		2, 1, // two 1s at stored (1,1),(2,1)
		0, 1, // EOB
	}
	img, err := Decode(buildBMP(4, 3, 8, 1, testPal, stream))
	if err != nil {
		t.Fatal(err)
	}
	// Stored row 1 = top-down row 1 (middle row of 3).
	if img.Pixels[1*4+1] != 1 || img.Pixels[1*4+2] != 1 {
		t.Fatalf("middle row %v", img.Pixels[4:8])
	}
	if img.Pixels[0] != 0 || img.Pixels[11] != 0 {
		t.Fatalf("unwritten pixels not zero: %v", img.Pixels)
	}
}

func TestRLE8RunCrossesRow(t *testing.T) {
	stream := []byte{5, 1, 0, 1} // width 4, run of 5
	_, err := Decode(buildBMP(4, 1, 8, 1, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "crosses row width") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8DeltaOutOfBounds(t *testing.T) {
	stream := []byte{0, 2, 9, 0, 0, 1}
	_, err := Decode(buildBMP(4, 2, 8, 1, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "out of image") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8TruncatedAbsolute(t *testing.T) {
	stream := []byte{0, 4, 1, 1} // declares 4 literals, only 2 present
	_, err := Decode(buildBMP(4, 1, 8, 1, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8MissingEOB(t *testing.T) {
	stream := []byte{2, 1, 0, 0, 1, 1} // ends cleanly but no EOB
	_, err := Decode(buildBMP(4, 2, 8, 1, testPal, stream))
	if err == nil || !strings.Contains(err.Error(), "end-of-bitmap") {
		t.Fatalf("got %v", err)
	}
}

func TestRLE8BadIndex(t *testing.T) {
	stream := []byte{2, 9, 0, 1}
	_, err := Decode(buildBMP(4, 1, 8, 1, testPal, stream))
	pe, ok := err.(*ParseError)
	if !ok || !strings.Contains(err.Error(), "palette index 9") {
		t.Fatalf("got %v", err)
	}
	if pe.Offset != int64(14+40+len(testPal)*4+1) {
		t.Fatalf("offset %d", pe.Offset)
	}
}

func TestRLE8NegativeHeightRejected(t *testing.T) {
	_, err := Decode(buildBMP(4, -2, 8, 1, testPal, []byte{0, 1}))
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("got %v", err)
	}
}

func TestUnsupportedFormats(t *testing.T) {
	if _, err := Decode(buildBMP(2, 2, 16, 0, nil, make([]byte, 16))); err == nil {
		t.Fatal("16-bit accepted")
	}
	if _, err := Decode(buildBMP(2, 2, 8, 2, testPal, make([]byte, 4))); err == nil {
		t.Fatal("RLE4 accepted")
	}
	bad := buildBMP(2, 2, 24, 0, nil, make([]byte, 16))
	bad[0] = 'X'
	if _, err := Decode(bad); err == nil {
		t.Fatal("bad signature accepted")
	}
}
