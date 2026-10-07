package hpack

import (
	"errors"
	"fmt"
)

var (
	// ErrIntegerTruncated means an integer representation ended before its
	// continuation bytes were fully consumed.
	ErrIntegerTruncated = errors.New("hpack: truncated integer")
	// ErrStringTruncated means a string literal did not contain all of the
	// bytes declared by its length prefix.
	ErrStringTruncated = errors.New("hpack: truncated string literal")
	// ErrHuffmanUnsupported means a string literal set the H bit. This
	// workbench only supports raw UTF-8 and rejects Huffman encoding.
	ErrHuffmanUnsupported = errors.New("hpack: Huffman encoding is not supported")
	// ErrInvalidUTF8 means a raw string literal was not valid UTF-8.
	ErrInvalidUTF8 = errors.New("hpack: string literal is not valid UTF-8")
	// ErrIndexOutOfRange means an indexed or indexed-name reference pointed
	// past the end of the static plus dynamic tables.
	ErrIndexOutOfRange = errors.New("hpack: index out of range")
	// ErrTableSizeUpdate means a size update exceeded the negotiated limit
	// or appeared after a header field within the same block.
	ErrTableSizeUpdate = errors.New("hpack: invalid dynamic table size update")
	// ErrInvalidRepresentation means the block contained bytes that could
	// not be parsed as any HPACK representation.
	ErrInvalidRepresentation = errors.New("hpack: invalid representation")
	// ErrFailedDirection means a block was processed on a direction that
	// previously failed and has not been explicitly reset.
	ErrFailedDirection = errors.New("hpack: direction is in failed state, reset required")
)

// BlockError locates a decode failure inside a stream of header blocks.
type BlockError struct {
	// Dir is "request" or "response".
	Dir string
	// Block is the 0-based sequence number of the failing block.
	Block int
	// Offset is the 0-based byte offset within the failing block.
	Offset int
	Err    error
}

func (e *BlockError) Error() string {
	return fmt.Sprintf("hpack: %s direction, block %d, byte %d: %v",
		e.Dir, e.Block, e.Offset, e.Err)
}

func (e *BlockError) Unwrap() error { return e.Err }

// offsetError wraps a primitive-level error with the current byte offset.
type offsetError struct {
	offset int
	err    error
}

func (e *offsetError) Error() string {
	return fmt.Sprintf("byte %d: %v", e.offset, e.err)
}

func (e *offsetError) Unwrap() error { return e.err }
