# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go library that removes embedded thumbnails from JPEG EXIF metadata while preserving all other metadata. It's part of the lightfile6 project ecosystem.

## Development Commands

```bash
# Run tests
go test -v ./

# Run tests with coverage
go test -v -cover ./

# Download dependencies
go mod download

# Run as CLI (though no main() exists in current implementation)
go run exifremovethumbnail.go -in input.jpg -out output.jpg
```

## Architecture and Key Implementation Details

### Core Function
```go
func ExifRemoveThumbnail(inputPath, outputPath string) (ExifRemoveThumbnailResult, error)
```

### Error Handling Strategy
- **FormatError**: Used for data/format issues (JPEG parsing failures, invalid structure)
- **Standard errors**: Used for system issues (file I/O, permissions)
- All errors wrapped with `%w` to maintain error chain for upstream `errors.Is` checking

### Implementation Approach
1. Parses JPEG segments sequentially without loading entire image
2. Finds APP1 (0xFFE1) segments containing EXIF data
3. Locates IFD1 (thumbnail) offset in TIFF structure
4. Sets IFD1 offset to 0 and truncates remaining data
5. Handles both big-endian and little-endian TIFF headers

### Key Design Decisions
- Pure Go implementation with no external dependencies in core code
- Bilingual comments (Japanese and English) throughout
- Returns detailed statistics about the operation
- Preserves all metadata except thumbnails

## Test Data Structure

The `testdata/` directory contains test images with various configurations:
- `actual_png.jpg` - PNG disguised as JPEG (format error test)
- `metadata_*.jpg` - Different metadata configurations
- `thumbnail_*.jpg` - With/without embedded thumbnails

## Integration with lightfile6 Ecosystem

This package follows lightfile6 conventions:
- Distinguishes between `DataError` (format issues) and system errors
- Uses `%w` error wrapping for error chain preservation
- Designed for batch processing scenarios where error categorization matters

## CI/CD Notes

GitHub Actions workflow runs on:
- Push/PR to main and develop branches
- Ubuntu latest with Go 1.22
- Standard `go test -v ./` execution