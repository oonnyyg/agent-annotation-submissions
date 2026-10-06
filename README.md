# bmp-rle-raster-intake

Offline intake tool for legacy BMP files. It decodes supported images
into a grayscale raster with a unified top-left origin, writes a binary
PGM plus a JSON summary, and reports corrupt files with the exact byte
offset of the problem. Pure Go standard library; no external image
tools are invoked.

## Usage

```
go run ./cmd/bmpintake config.json
```

Config file:

```json
{
  "input": "scan.bmp",
  "pgm_output": "scan.pgm",
  "summary_output": "scan.json",
  "max_pixels": 4000000,
  "crop": { "x": 0, "y": 0, "width": 640, "height": 480 }
}
```

- `input`, `pgm_output`, `summary_output` are required paths.
- `max_pixels` (required, positive) caps `width * height` of the
  decoded image; larger images are rejected before cropping.
- `crop` is optional and defaults to the whole image. It is a
  half-open rectangle `[x, x+width) x [y, y+height)` with origin at
  the top-left corner; rectangles outside the image are rejected.

Outputs are published only after every validation succeeds, and
existing output files are never overwritten (`O_EXCL`).

## Supported formats

- 40-byte `BITMAPINFOHEADER` only; other header sizes are rejected.
- 24-bit uncompressed (`BI_RGB`), positive (bottom-up) or negative
  (top-down) height. Rows are padded to 4 bytes; pixels are BGR.
- 8-bit uncompressed (`BI_RGB`), either height sign. Palette indices
  are validated against the palette size (`biClrUsed`, default 256).
- 8-bit RLE8 (`BI_RLE8`), positive height only. Encoded runs, absolute
  runs (with the even-byte pad), end-of-line, end-of-bitmap and delta
  commands are supported. Pixels never written default to palette
  index 0.

Everything else (other bit depths, RLE4, bitfields, other header
versions, zero/invalid dimensions, pixel offsets overlapping headers
or past EOF) is rejected with a byte-offset error.

## Corruption handling

Errors are reported as `bmp: byte offset N: message`. The decoder
refuses to mask damage with zero padding:

- runs and absolute segments may not cross the row width;
- deltas may not move the cursor outside the image;
- a stream that ends mid-command is reported as truncated, while a
  stream that simply runs out of bytes without an end-of-bitmap marker
  is reported as missing the marker;
- out-of-range palette indices and out-of-bounds cursors are rejected
  at the offending byte.

## Output protocol

- PGM: binary `P5`, max value 255, one byte per pixel, row-major from
  the top-left corner of the cropped rectangle.
- Grayscale: `(299*R + 587*G + 114*B + 500) / 1000` (integer
  division), range 0-255.
- Summary JSON: `source_width`, `source_height`, cropped `width` and
  `height`, and the complete 256-bin grayscale `histogram` of the
  cropped image.

## Tests

```
go test ./...
```

Covers row padding, top-down/bottom-up orientation, RLE8 runs,
absolute-mode padding, deltas, unwritten-pixel defaults, truncation
vs. missing-marker distinction, palette bounds, crop bounds, grayscale
rounding, the pixel limit, and no-overwrite publishing.
