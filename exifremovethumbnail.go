// Package exifremovethumbnail provides functions to remove embedded thumbnails from JPEG EXIF metadata.
// It preserves other EXIF data and outputs a new JPEG file without the thumbnail.
package exifremovethumbnail

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// ExifRemoveThumbnailResult is the result of thumbnail removal from a JPEG file.
// HadThumbnail is true if the original image contained a thumbnail.
// BeforeSize and AfterSize are the file sizes before and after processing.
// ThumbnailSize is the size of the removed thumbnail in bytes (0 if none).
type ExifRemoveThumbnailResult struct {
	HadThumbnail  bool
	BeforeSize    int64
	AfterSize     int64
	ThumbnailSize int64
}

// FormatError represents an error due to invalid or unsupported file format.
type FormatError struct {
	msg string
}

func (e *FormatError) Error() string {
	return e.msg
}

// ExifRemoveThumbnailBytes removes the EXIF thumbnail from JPEG data in memory.
// It returns the modified JPEG data and information about the operation.
// If no thumbnail exists, HadThumbnail will be false.
func ExifRemoveThumbnailBytes(inputData []byte) ([]byte, ExifRemoveThumbnailResult, error) {
	var result ExifRemoveThumbnailResult
	result.BeforeSize = int64(len(inputData))

	const markerSOI = 0xFFD8
	const markerAPP1 = 0xFFE1
	const markerSOS = 0xFFDA

	if len(inputData) < 2 || binary.BigEndian.Uint16(inputData[0:2]) != markerSOI {
		return nil, result, &FormatError{"not a valid JPEG file"}
	}

	output := &bytes.Buffer{}
	reader := bytes.NewReader(inputData)
	soi := make([]byte, 2)
	reader.Read(soi)
	output.Write(soi)

	thumbnailSize := int64(0)
	foundThumbnail := false

	for {
		var marker uint16
		err := binary.Read(reader, binary.BigEndian, &marker)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, result, fmt.Errorf("failed to read marker: %w", err)
		}
		if marker&0xFF00 != 0xFF00 {
			return nil, result, &FormatError{"invalid JPEG marker"}
		}
		if marker == markerSOS {
			binary.Write(output, binary.BigEndian, marker)
			remaining, _ := io.ReadAll(reader)
			output.Write(remaining)
			break
		}
		var segmentLength uint16
		err = binary.Read(reader, binary.BigEndian, &segmentLength)
		if err != nil {
			return nil, result, fmt.Errorf("failed to read segment length: %w", err)
		}
		segmentData := make([]byte, segmentLength-2)
		_, err = io.ReadFull(reader, segmentData)
		if err != nil {
			return nil, result, fmt.Errorf("failed to read segment data: %w", err)
		}
		if marker == markerAPP1 && len(segmentData) > 6 && string(segmentData[0:6]) == "Exif\x00\x00" {
			modifiedExif, hadThumb, thumbSize, err := removeThumbnailFromExif(segmentData)
			if err != nil {
				return nil, result, &FormatError{"failed to remove EXIF thumbnail: " + err.Error()}
			}
			if hadThumb {
				foundThumbnail = true
				thumbnailSize = thumbSize
			}
			binary.Write(output, binary.BigEndian, marker)
			binary.Write(output, binary.BigEndian, uint16(len(modifiedExif)+2))
			output.Write(modifiedExif)
		} else {
			binary.Write(output, binary.BigEndian, marker)
			binary.Write(output, binary.BigEndian, segmentLength)
			output.Write(segmentData)
		}
	}
	outputData := output.Bytes()
	result.AfterSize = int64(len(outputData))
	result.HadThumbnail = foundThumbnail
	result.ThumbnailSize = thumbnailSize
	return outputData, result, nil
}

// ExifRemoveThumbnail removes the EXIF thumbnail from a JPEG image at inputPath and writes the result to outputPath.
// It returns information about the operation and an error if the process fails.
func ExifRemoveThumbnail(inputPath, outputPath string) (ExifRemoveThumbnailResult, error) {
	inputData, err := os.ReadFile(inputPath)
	if err != nil {
		return ExifRemoveThumbnailResult{}, fmt.Errorf("failed to read input file: %w", err)
	}
	
	outputData, result, err := ExifRemoveThumbnailBytes(inputData)
	if err != nil {
		return result, err
	}
	
	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return result, fmt.Errorf("failed to write output file: %w", err)
	}
	
	return result, nil
}

// removeThumbnailFromExif removes thumbnail from EXIF segment data
func removeThumbnailFromExif(exifData []byte) ([]byte, bool, int64, error) {
	if len(exifData) < 6 || string(exifData[0:6]) != "Exif\x00\x00" {
		return exifData, false, 0, fmt.Errorf("invalid EXIF header")
	}
	// Simple implementation: just set IFD1 offset to 0
	// TIFF header starts from byte 6
	pos := 6
	if len(exifData) < pos+8 {
		return exifData, false, 0, fmt.Errorf("invalid TIFF header")
	}
	byteOrder := binary.BigEndian.Uint16(exifData[pos : pos+2])
	littleEndian := byteOrder == 0x4949
	var readUint16 func([]byte) uint16
	var readUint32 func([]byte) uint32
	if littleEndian {
		readUint16 = func(b []byte) uint16 { return binary.LittleEndian.Uint16(b) }
		readUint32 = func(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }
	} else {
		readUint16 = func(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
		readUint32 = func(b []byte) uint32 { return binary.BigEndian.Uint32(b) }
	}
	ifd0Offset := int(readUint32(exifData[pos+4 : pos+8]))
	ifd0Pos := pos + ifd0Offset
	if len(exifData) < ifd0Pos+2 {
		return exifData, false, 0, fmt.Errorf("invalid IFD0")
	}
	entryCount := int(readUint16(exifData[ifd0Pos : ifd0Pos+2]))
	ifd1OffsetPos := ifd0Pos + 2 + entryCount*12
	if len(exifData) < ifd1OffsetPos+4 {
		return exifData, false, 0, fmt.Errorf("invalid IFD1 offset")
	}
	ifd1Offset := int(readUint32(exifData[ifd1OffsetPos : ifd1OffsetPos+4]))
	if ifd1Offset == 0 {
		return exifData, false, 0, nil
	}
	// Estimate thumbnail size: from IFD1 start to end of EXIF data
	thumbStart := pos + ifd1Offset
	thumbSize := int64(len(exifData) - thumbStart)
	// Set IFD1 offset to 0
	result := make([]byte, len(exifData))
	copy(result, exifData)
	if littleEndian {
		binary.LittleEndian.PutUint32(result[ifd1OffsetPos:], 0)
	} else {
		binary.BigEndian.PutUint32(result[ifd1OffsetPos:], 0)
	}
	// Remove data after IFD1
	if thumbStart < len(result) {
		result = result[:thumbStart]
	}
	return result, true, thumbSize, nil
}
