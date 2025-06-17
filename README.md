# go-exif-remove-thumbnail

[日本語 README はこちら](README.ja.md)

A simple Go library and CLI tool to remove embedded thumbnails from JPEG EXIF metadata.

## Features

- Remove EXIF thumbnail from JPEG images
- CLI and library usage
- No external dependencies (pure Go)

## Usage

### CLI

```sh
go run exifremovethumbnail.go -in input.jpg -out output.jpg
```

### As a Library

#### File-based operations

```go
import "github.com/ideamans/go-exif-remove-thumbnail"

result, err := exifremovethumbnail.ExifRemoveThumbnail(inputPath, outputPath)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Thumbnail removed: %v, saved %d bytes\n", 
    result.HadThumbnail, result.ThumbnailSize)
```

#### Memory-based operations

```go
import "github.com/ideamans/go-exif-remove-thumbnail"

// Read JPEG data
inputData, err := os.ReadFile("input.jpg")
if err != nil {
    log.Fatal(err)
}

// Remove thumbnail from memory
outputData, result, err := exifremovethumbnail.ExifRemoveThumbnailBytes(inputData)
if err != nil {
    log.Fatal(err)
}

// Write the result
err = os.WriteFile("output.jpg", outputData, 0644)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Thumbnail removed: %v, saved %d bytes\n", 
    result.HadThumbnail, result.ThumbnailSize)
```

## Test

```sh
go test
```

## Test Images

Test images are in the `testdata/` directory.

## Return Value Structure

The library function `ExifRemoveThumbnail` returns an `ExifRemoveThumbnailResult` struct:

```go
// Result of thumbnail removal
 type ExifRemoveThumbnailResult struct {
     HadThumbnail  bool   // Whether the original image had a thumbnail
     BeforeSize    int64  // File size before processing
     AfterSize     int64  // File size after processing
     ThumbnailSize int64  // Size of the removed thumbnail
 }
```

- `HadThumbnail`: true if the original image contained a thumbnail
- `BeforeSize`: input image size in bytes
- `AfterSize`: output image size in bytes
- `ThumbnailSize`: size of the removed thumbnail in bytes (0 if none)

## License

MIT License
