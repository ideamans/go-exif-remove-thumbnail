# 仕様

このプログラムは、JPEG 画像の Exif からサムネイルを削除するツールです。サムネイル意外の Exif データ保持します。

## パッケージ名

`exifremovethumbnail`

## 関数シグネチャ

```go
func ExifRemoveThumbnail(inputPath, outputPath string) (ExifRemoveThumbnailResult, error)
```

## エラータイプ

- `FormatError` - 処理としては正しいがデータフォーマットに問題がある場合に使用するエラー。
- `Error` - ファイルが開けないなど予期しない一般エラー。

## 戻り値

- 情報 ExifRemoveThumbnailResult
  - HadThumbnail 元画像のサムネイル有無
  - BeforeSize 元画像のファイルサイズ
  - AfterSize 出力画像のファイルサイズ
  - ThumbnailSize サムネイルのファイルサイズ
- エラー Error または FormatError

# 使用してよいパッケージ

- github.com/dsoprea/go-exif/v3
- github.com/dsoprea/go-jpeg-image-structure/v2

# 事前に作成した原案

```go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	markerSOI  = 0xFFD8
	markerAPP1 = 0xFFE1
	markerSOS  = 0xFFDA
)

type ExifReader struct {
	data []byte
	pos  int
}

func (r *ExifReader) readUint16() uint16 {
	v := binary.BigEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return v
}

func (r *ExifReader) readUint32(littleEndian bool) uint32 {
	var v uint32
	if littleEndian {
		v = binary.LittleEndian.Uint32(r.data[r.pos:])
	} else {
		v = binary.BigEndian.Uint32(r.data[r.pos:])
	}
	r.pos += 4
	return v
}

func removeThumbnailFromExif(exifData []byte) ([]byte, error) {
	reader := &ExifReader{data: exifData, pos: 0}

	// Exifヘッダーの確認
	if len(exifData) < 6 || string(exifData[0:6]) != "Exif\x00\x00" {
		return nil, fmt.Errorf("invalid Exif header")
	}
	reader.pos = 6

	// TIFF headerの読み取り
	tiffStart := reader.pos
	byteOrder := reader.readUint16()
	littleEndian := byteOrder == 0x4949 // "II"

	// 0x002A のチェック
	magic := reader.readUint16()
	if magic != 0x002A {
		return nil, fmt.Errorf("invalid TIFF magic number")
	}

	// IFD0へのオフセット
	ifd0Offset := reader.readUint32(littleEndian)
	reader.pos = tiffStart + int(ifd0Offset)

	// IFD0のエントリ数を読む
	entryCount := reader.readUint16()
	reader.pos += int(entryCount) * 12 // 各エントリは12バイト

	// IFD1へのオフセット
	ifd1Offset := reader.readUint32(littleEndian)
	if ifd1Offset == 0 {
		// サムネイルがない場合はそのまま返す
		return exifData, nil
	}

	// IFD1の位置を0にしてサムネイルを削除
	ifd1OffsetPos := reader.pos - 4
	result := make([]byte, len(exifData))
	copy(result, exifData)

	// IFD1オフセットを0に設定
	if littleEndian {
		binary.LittleEndian.PutUint32(result[ifd1OffsetPos:], 0)
	} else {
		binary.BigEndian.PutUint32(result[ifd1OffsetPos:], 0)
	}

	// IFD1以降のデータを削除
	actualExifEnd := tiffStart + int(ifd1Offset)
	if actualExifEnd < len(result) {
		result = result[:actualExifEnd]
	}

	return result, nil
}

func processJPEG(inputPath, outputPath string) error {
	// 入力ファイルを読み込む
	inputData, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// JPEGマーカーの確認
	if len(inputData) < 2 || binary.BigEndian.Uint16(inputData[0:2]) != markerSOI {
		return fmt.Errorf("not a valid JPEG file")
	}

	output := &bytes.Buffer{}
	reader := bytes.NewReader(inputData)

	// SOIマーカーをコピー
	soi := make([]byte, 2)
	reader.Read(soi)
	output.Write(soi)

	for {
		// マーカーを読む
		var marker uint16
		err := binary.Read(reader, binary.BigEndian, &marker)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read marker: %w", err)
		}

		// マーカーが0xFFで始まることを確認
		if marker&0xFF00 != 0xFF00 {
			return fmt.Errorf("invalid marker: %04X", marker)
		}

		// SOSマーカー以降は全てコピー
		if marker == markerSOS {
			binary.Write(output, binary.BigEndian, marker)
			remaining, _ := io.ReadAll(reader)
			output.Write(remaining)
			break
		}

		// セグメント長を読む
		var segmentLength uint16
		err = binary.Read(reader, binary.BigEndian, &segmentLength)
		if err != nil {
			return fmt.Errorf("failed to read segment length: %w", err)
		}

		// セグメントデータを読む（長さには自身の2バイトが含まれる）
		segmentData := make([]byte, segmentLength-2)
		_, err = io.ReadFull(reader, segmentData)
		if err != nil {
			return fmt.Errorf("failed to read segment data: %w", err)
		}

		// APP1（Exif）セグメントの場合
		if marker == markerAPP1 && len(segmentData) > 6 && string(segmentData[0:6]) == "Exif\x00\x00" {
			// サムネイルを削除
			modifiedExif, err := removeThumbnailFromExif(segmentData)
			if err != nil {
				fmt.Printf("Warning: failed to remove thumbnail: %v\n", err)
				// エラーが発生した場合は元のデータを使用
				binary.Write(output, binary.BigEndian, marker)
				binary.Write(output, binary.BigEndian, segmentLength)
				output.Write(segmentData)
			} else {
				// 修正されたExifデータを書き込む
				binary.Write(output, binary.BigEndian, marker)
				binary.Write(output, binary.BigEndian, uint16(len(modifiedExif)+2))
				output.Write(modifiedExif)
			}
		} else {
			// その他のセグメントはそのままコピー
			binary.Write(output, binary.BigEndian, marker)
			binary.Write(output, binary.BigEndian, segmentLength)
			output.Write(segmentData)
		}
	}

	// 出力ファイルに書き込む
	err = os.WriteFile(outputPath, output.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input.jpg> <output.jpg>\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]

	err := processJPEG(inputPath, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully removed thumbnail from %s and saved to %s\n", inputPath, outputPath)
}
```

# testdata の内容

- `actual_png.jpg` - 実際は PNG フォーマットの JPEG 画像

```json
[
  {
    "format": "jpeg",
    "path": "test_original.jpg",
    "jp": "JPEG元画像（高品質、豊富なメタデータ、多様なコンテンツ）",
    "en": "Original JPEG image (high quality, rich metadata, diverse content)"
  },
  {
    "format": "png",
    "path": "test_original.png",
    "jp": "PNG元画像（RGBA、透明度、メタデータチャンク）",
    "en": "Original PNG image (RGBA, transparency, metadata chunks)"
  },
  {
    "format": "gif",
    "path": "test_original.gif",
    "jp": "GIF元画像（アニメーション、多様な動的要素）",
    "en": "Original GIF image (animation, diverse dynamic elements)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/colorspace_rgb.jpg",
    "jp": "RGB色空間での保存",
    "en": "Saved in RGB color space"
  },
  {
    "format": "jpeg",
    "path": "jpeg/colorspace_cmyk.jpg",
    "jp": "CMYK色空間での保存（印刷用）",
    "en": "Saved in CMYK color space (for printing)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/colorspace_grayscale.jpg",
    "jp": "グレースケール（白黒）での保存",
    "en": "Saved in grayscale (black and white)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/encoding_baseline.jpg",
    "jp": "ベースラインJPEG（標準形式）",
    "en": "Baseline JPEG (standard format)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/encoding_progressive.jpg",
    "jp": "プログレッシブJPEG（段階的表示対応）",
    "en": "Progressive JPEG (supports gradual display)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/thumbnail_none.jpg",
    "jp": "サムネイル画像なし",
    "en": "No embedded thumbnail"
  },
  {
    "format": "jpeg",
    "path": "jpeg/thumbnail_embedded.jpg",
    "jp": "サムネイル画像埋め込み",
    "en": "Embedded thumbnail image"
  },
  {
    "format": "jpeg",
    "path": "jpeg/quality_20.jpg",
    "jp": "低品質（高圧縮、ファイルサイズ小）",
    "en": "Low quality (high compression, small file size)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/quality_50.jpg",
    "jp": "中品質（バランス型）",
    "en": "Medium quality (balanced)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/quality_80.jpg",
    "jp": "高品質（低圧縮）",
    "en": "High quality (low compression)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/quality_95.jpg",
    "jp": "最高品質（ほぼ無劣化）",
    "en": "Highest quality (nearly lossless)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/subsampling_444.jpg",
    "jp": "4:4:4サブサンプリング（最高品質）",
    "en": "4:4:4 subsampling (highest quality)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/subsampling_422.jpg",
    "jp": "4:2:2サブサンプリング（中品質）",
    "en": "4:2:2 subsampling (medium quality)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/subsampling_420.jpg",
    "jp": "4:2:0サブサンプリング（高圧縮）",
    "en": "4:2:0 subsampling (high compression)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/metadata_none.jpg",
    "jp": "メタデータなし（軽量化）",
    "en": "No metadata (lightweight)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/metadata_basic_exif.jpg",
    "jp": "基本的なEXIF情報のみ",
    "en": "Basic EXIF information only"
  },
  {
    "format": "jpeg",
    "path": "jpeg/metadata_gps.jpg",
    "jp": "GPS位置情報付きEXIF",
    "en": "EXIF with GPS location data"
  },
  {
    "format": "jpeg",
    "path": "jpeg/metadata_full_exif.jpg",
    "jp": "完全なEXIF情報（撮影情報等）",
    "en": "Complete EXIF information (shooting data, etc.)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/icc_none.jpg",
    "jp": "カラープロファイルなし",
    "en": "No color profile"
  },
  {
    "format": "jpeg",
    "path": "jpeg/icc_srgb.jpg",
    "jp": "sRGBカラープロファイル（Web標準）",
    "en": "sRGB color profile (web standard)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/icc_adobergb.jpg",
    "jp": "Adobe RGBカラープロファイル（広色域）",
    "en": "Adobe RGB color profile (wide gamut)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/orientation_1.jpg",
    "jp": "通常の向き（Top-left）",
    "en": "Normal orientation (Top-left)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/orientation_3.jpg",
    "jp": "180度回転（Bottom-right）",
    "en": "Rotated 180 degrees (Bottom-right)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/orientation_6.jpg",
    "jp": "時計回りに90度回転（Right-top）",
    "en": "Rotated 90 degrees clockwise (Right-top)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/orientation_8.jpg",
    "jp": "反時計回りに90度回転（Left-bottom）",
    "en": "Rotated 90 degrees counter-clockwise (Left-bottom)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/dpi_jfif_units0.jpg",
    "jp": "JFIF units:0 (縦横比のみ)",
    "en": "JFIF units:0 (aspect ratio only)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/dpi_jfif_72dpi.jpg",
    "jp": "JFIF units:1 72DPI",
    "en": "JFIF units:1 72DPI"
  },
  {
    "format": "jpeg",
    "path": "jpeg/dpi_jfif_200dpi.jpg",
    "jp": "JFIF units:1 200DPI",
    "en": "JFIF units:1 200DPI"
  },
  {
    "format": "jpeg",
    "path": "jpeg/dpi_exif_72dpi.jpg",
    "jp": "EXIF指定 72DPI",
    "en": "EXIF specified 72DPI"
  },
  {
    "format": "jpeg",
    "path": "jpeg/dpi_exif_200dpi.jpg",
    "jp": "EXIF指定 200DPI",
    "en": "EXIF specified 200DPI"
  },
  {
    "format": "jpeg",
    "path": "jpeg/critical_cmyk_lowquality.jpg",
    "jp": "CMYK色空間と低品質の組み合わせ（高圧縮）",
    "en": "CMYK color space with low quality (high compression)"
  },
  {
    "format": "jpeg",
    "path": "jpeg/critical_progressive_fullmeta.jpg",
    "jp": "プログレッシブ形式と完全メタデータの組み合わせ",
    "en": "Progressive format with complete metadata"
  },
  {
    "format": "jpeg",
    "path": "jpeg/critical_thumbnail_progressive.jpg",
    "jp": "サムネイル埋め込みとプログレッシブの組み合わせ",
    "en": "Embedded thumbnail with progressive format"
  },
  {
    "format": "jpeg",
    "path": "jpeg/critical_orientation_metadata.jpg",
    "jp": "回転orientation情報と複雑メタデータの組み合わせ",
    "en": "Rotated orientation with complex metadata"
  },
  {
    "format": "jpeg",
    "path": "jpeg/critical_jfif_exif_dpi.jpg",
    "jp": "JFIF units:1 72DPIとEXIF 200DPIの併存",
    "en": "JFIF units:1 72DPI with EXIF 200DPI conflict"
  },
  {
    "format": "png",
    "path": "png/colortype_grayscale.png",
    "jp": "グレースケール（白黒画像）",
    "en": "Grayscale (black and white image)"
  },
  {
    "format": "png",
    "path": "png/colortype_palette.png",
    "jp": "パレットカラー（256色まで）",
    "en": "Palette color (up to 256 colors)"
  },
  {
    "format": "png",
    "path": "png/colortype_rgb.png",
    "jp": "RGB（透明度なし）",
    "en": "RGB (no transparency)"
  },
  {
    "format": "png",
    "path": "png/colortype_rgba.png",
    "jp": "RGBA（透明度あり）",
    "en": "RGBA (with transparency)"
  },
  {
    "format": "png",
    "path": "png/colortype_grayscale_alpha.png",
    "jp": "グレースケール+透明度",
    "en": "Grayscale with transparency"
  },
  {
    "format": "png",
    "path": "png/interlace_none.png",
    "jp": "インターレースなし（通常）",
    "en": "No interlace (standard)"
  },
  {
    "format": "png",
    "path": "png/interlace_adam7.png",
    "jp": "Adam7インターレース（段階的表示）",
    "en": "Adam7 interlace (progressive display)"
  },
  {
    "format": "png",
    "path": "png/depth_1bit.png",
    "jp": "1ビット深度（白黒のみ）",
    "en": "1-bit depth (black and white only)"
  },
  {
    "format": "png",
    "path": "png/depth_8bit.png",
    "jp": "8ビット深度（標準）",
    "en": "8-bit depth (standard)"
  },
  {
    "format": "png",
    "path": "png/depth_16bit.png",
    "jp": "16ビット深度（高精度）",
    "en": "16-bit depth (high precision)"
  },
  {
    "format": "png",
    "path": "png/compression_0.png",
    "jp": "圧縮なし（最大ファイルサイズ）",
    "en": "No compression (maximum file size)"
  },
  {
    "format": "png",
    "path": "png/compression_6.png",
    "jp": "標準圧縮（デフォルト）",
    "en": "Standard compression (default)"
  },
  {
    "format": "png",
    "path": "png/compression_9.png",
    "jp": "最大圧縮（最小ファイルサイズ）",
    "en": "Maximum compression (minimum file size)"
  },
  {
    "format": "png",
    "path": "png/alpha_opaque.png",
    "jp": "完全不透明",
    "en": "Completely opaque"
  },
  {
    "format": "png",
    "path": "png/alpha_semitransparent.png",
    "jp": "半透明（部分的透明度）",
    "en": "Semi-transparent (partial transparency)"
  },
  {
    "format": "png",
    "path": "png/alpha_transparent.png",
    "jp": "透明領域あり",
    "en": "Has transparent areas"
  },
  {
    "format": "png",
    "path": "png/filter_none.png",
    "jp": "フィルターなし",
    "en": "No filter"
  },
  {
    "format": "png",
    "path": "png/filter_sub.png",
    "jp": "Subフィルター（水平予測）",
    "en": "Sub filter (horizontal prediction)"
  },
  {
    "format": "png",
    "path": "png/filter_up.png",
    "jp": "Upフィルター（垂直予測）",
    "en": "Up filter (vertical prediction)"
  },
  {
    "format": "png",
    "path": "png/filter_average.png",
    "jp": "Averageフィルター（平均予測）",
    "en": "Average filter (average prediction)"
  },
  {
    "format": "png",
    "path": "png/filter_paeth.png",
    "jp": "Paethフィルター（複合予測）",
    "en": "Paeth filter (complex prediction)"
  },
  {
    "format": "png",
    "path": "png/metadata_none.png",
    "jp": "メタデータなし",
    "en": "No metadata"
  },
  {
    "format": "png",
    "path": "png/metadata_text.png",
    "jp": "テキストメタデータ",
    "en": "Text metadata"
  },
  {
    "format": "png",
    "path": "png/metadata_compressed.png",
    "jp": "圧縮テキストメタデータ",
    "en": "Compressed text metadata"
  },
  {
    "format": "png",
    "path": "png/metadata_international.png",
    "jp": "国際化テキスト（UTF-8）",
    "en": "International text (UTF-8)"
  },
  {
    "format": "png",
    "path": "png/chunk_gamma.png",
    "jp": "ガンマ補正情報",
    "en": "Gamma correction information"
  },
  {
    "format": "png",
    "path": "png/chunk_background.png",
    "jp": "背景色指定",
    "en": "Background color specification"
  },
  {
    "format": "png",
    "path": "png/chunk_transparency.png",
    "jp": "透明色指定",
    "en": "Transparent color specification"
  },
  {
    "format": "png",
    "path": "png/critical_16bit_palette.png",
    "jp": "16ビットからパレットへの変換（大幅な色情報損失）",
    "en": "16-bit to palette conversion (significant color information loss)"
  },
  {
    "format": "png",
    "path": "png/critical_alpha_grayscale.png",
    "jp": "RGBAからグレースケール+透明度への変換",
    "en": "RGBA to grayscale with alpha conversion"
  },
  {
    "format": "png",
    "path": "png/critical_maxcompression_paeth.png",
    "jp": "最大圧縮とPaethフィルターの組み合わせ",
    "en": "Maximum compression with Paeth filter combination"
  },
  {
    "format": "png",
    "path": "png/critical_interlace_highres.png",
    "jp": "インターレースと高解像度の組み合わせ",
    "en": "Interlace with high resolution combination"
  },
  {
    "format": "gif",
    "path": "gif/frames_single.gif",
    "jp": "静止画GIF（1フレーム）",
    "en": "Static GIF (1 frame)"
  },
  {
    "format": "gif",
    "path": "gif/frames_short.gif",
    "jp": "短いアニメーション（5フレーム）",
    "en": "Short animation (5 frames)"
  },
  {
    "format": "gif",
    "path": "gif/frames_medium.gif",
    "jp": "中程度アニメーション（10フレーム）",
    "en": "Medium animation (10 frames)"
  },
  {
    "format": "gif",
    "path": "gif/frames_long.gif",
    "jp": "長いアニメーション（20フレーム）",
    "en": "Long animation (20 frames)"
  },
  {
    "format": "gif",
    "path": "gif/fps_slow.gif",
    "jp": "低フレームレート（5 FPS）",
    "en": "Low frame rate (5 FPS)"
  },
  {
    "format": "gif",
    "path": "gif/fps_normal.gif",
    "jp": "標準フレームレート（10 FPS）",
    "en": "Normal frame rate (10 FPS)"
  },
  {
    "format": "gif",
    "path": "gif/fps_fast.gif",
    "jp": "高フレームレート（25 FPS）",
    "en": "High frame rate (25 FPS)"
  },
  {
    "format": "gif",
    "path": "gif/palette_2colors.gif",
    "jp": "2色パレット（最小）",
    "en": "2-color palette (minimum)"
  },
  {
    "format": "gif",
    "path": "gif/palette_16colors.gif",
    "jp": "16色パレット",
    "en": "16-color palette"
  },
  {
    "format": "gif",
    "path": "gif/palette_256colors.gif",
    "jp": "256色パレット（最大）",
    "en": "256-color palette (maximum)"
  },
  {
    "format": "gif",
    "path": "gif/dither_nodither.gif",
    "jp": "ディザリングなし",
    "en": "No dithering"
  },
  {
    "format": "gif",
    "path": "gif/dither_dithered.gif",
    "jp": "Floyd-Steinbergディザリング",
    "en": "Floyd-Steinberg dithering"
  },
  {
    "format": "gif",
    "path": "gif/optimize_noopt.gif",
    "jp": "最適化なし",
    "en": "No optimization"
  },
  {
    "format": "gif",
    "path": "gif/optimize_optimized.gif",
    "jp": "基本最適化（フレーム最適化）",
    "en": "Basic optimization (frame optimization)"
  },
  {
    "format": "gif",
    "path": "gif/loop_loop_infinite.gif",
    "jp": "無限ループ",
    "en": "Infinite loop"
  },
  {
    "format": "gif",
    "path": "gif/loop_loop_once.gif",
    "jp": "1回再生のみ",
    "en": "Play once only"
  },
  {
    "format": "gif",
    "path": "gif/loop_loop_3times.gif",
    "jp": "3回ループ",
    "en": "Loop 3 times"
  },
  {
    "format": "gif",
    "path": "gif/critical_fast_256colors_long.gif",
    "jp": "高フレームレート+大パレット+長時間（大ファイル）",
    "en": "High frame rate + large palette + long duration (large file)"
  },
  {
    "format": "gif",
    "path": "gif/critical_dither_smallpalette.gif",
    "jp": "ディザリング+小パレット（品質劣化）",
    "en": "Dithering + small palette (quality degradation)"
  },
  {
    "format": "gif",
    "path": "gif/critical_noopt_manyframes.gif",
    "jp": "最適化なし+多フレーム（非効率）",
    "en": "No optimization + many frames (inefficient)"
  }
]
```

# メタデータの判定プログラム例

以下のプログラムでは、Exif データの有無をチェックしています。テストの参考にしてください。

```go
package jpeg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ideamans/lightfile6-core/types"
)

func TestMetadata(t *testing.T) {
	tempDir := t.TempDir()

	cases := []struct {
		name              string
		file              string
		expectedAbortType string
		expectEXIF        bool // true if EXIF should be present
		expectGPS         bool // true if GPS data should be present
		description       string
	}{
		{
			name:              "Basic EXIF",
			file:              "metadata_basic_exif.jpg",
			expectedAbortType: types.AbortTypeNothing, // Should succeed with PSNR check skipped
			expectEXIF:        true,                   // Should have basic EXIF
			expectGPS:         true,                   // Actually has GPS data
			description:       "JPEG with basic EXIF metadata",
		},
		{
			name:              "GPS metadata",
			file:              "metadata_gps.jpg",
			expectedAbortType: types.AbortTypeNothing, // Should succeed with PSNR check skipped
			expectEXIF:        true,                   // Should have EXIF
			expectGPS:         true,                   // Should have GPS data
			description:       "JPEG with GPS location data",
		},
		{
			name:              "Full EXIF",
			file:              "metadata_full_exif.jpg",
			expectedAbortType: types.AbortTypeNothing, // Should succeed with PSNR check skipped
			expectEXIF:        true,                   // Should have comprehensive EXIF
			expectGPS:         true,                   // Actually has GPS data
			description:       "JPEG with comprehensive EXIF metadata",
		},
		{
			name:              "No metadata",
			file:              "metadata_none.jpg",
			expectedAbortType: types.AbortTypeNothing, // Should succeed with PSNR check skipped
			expectEXIF:        false,                  // Should not have EXIF
			expectGPS:         false,                  // Should not have GPS
			description:       "JPEG without any metadata",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputPath := filepath.Join(".", "testdata", "variations", tc.file)
			outputPath := filepath.Join(tempDir, tc.file)

			t.Logf("Testing: %s", tc.description)

			// Check metadata in original file
			originalHasEXIF, originalHasGPS := checkMetadata(t, inputPath)
			t.Logf("Original file metadata: EXIF=%v, GPS=%v", originalHasEXIF, originalHasGPS)

			// Verify our expectations match reality for the original file
			if originalHasEXIF != tc.expectEXIF {
				t.Errorf("Expected original EXIF presence %v, but got %v", tc.expectEXIF, originalHasEXIF)
			}
			if originalHasGPS != tc.expectGPS {
				t.Errorf("Expected original GPS presence %v, but got %v", tc.expectGPS, originalHasGPS)
			}

			option := &OptimizeJpegOption{}
			option.Quality = "medium"
			option.SkipPsnrAssertion = true

			result := Optimize(inputPath, outputPath, option)

			// Check expected abort type
			if result.AbortType != tc.expectedAbortType {
				t.Errorf("Expected AbortType %q, got %q", tc.expectedAbortType, result.AbortType)
				if result.AbortDetail != nil {
					t.Logf("AbortDetail: %v", result.AbortDetail)
				}
			}

			// Log optimization details regardless of result
			t.Logf("Optimization result: %d -> %d bytes", result.BeforeSize, result.AfterSize)
			t.Logf("Quality: %d, SSIM: %.6f, PSNR: %.2f", result.Quality, result.Ssim, result.FinalPsnr)
			t.Logf("AbortType: %s", result.AbortType)

			// Only check metadata preservation if optimization was successful
			if result.AbortType == types.AbortTypeNothing {
				// Check that output file exists
				if _, err := os.Stat(outputPath); os.IsNotExist(err) {
					t.Error("Output file was not created")
					return
				}

				// Check metadata in optimized file
				optimizedHasEXIF, optimizedHasGPS := checkMetadata(t, outputPath)
				t.Logf("Optimized file metadata: EXIF=%v, GPS=%v", optimizedHasEXIF, optimizedHasGPS)

				// Check metadata preservation/removal
				if originalHasEXIF && !optimizedHasEXIF {
					t.Logf("EXIF metadata was removed during optimization")
				} else if originalHasEXIF && optimizedHasEXIF {
					t.Logf("EXIF metadata was preserved during optimization")
				} else if !originalHasEXIF && optimizedHasEXIF {
					t.Logf("EXIF metadata was added during optimization (unexpected)")
				}

				if originalHasGPS && !optimizedHasGPS {
					t.Logf("GPS metadata was removed during optimization")
				} else if originalHasGPS && optimizedHasGPS {
					t.Logf("GPS metadata was preserved during optimization")
				} else if !originalHasGPS && optimizedHasGPS {
					t.Logf("GPS metadata was added during optimization (unexpected)")
				}

				// Log metadata preservation summary
				exifPreserved := (originalHasEXIF && optimizedHasEXIF) || (!originalHasEXIF && !optimizedHasEXIF)
				gpsPreserved := (originalHasGPS && optimizedHasGPS) || (!originalHasGPS && !optimizedHasGPS)
				t.Logf("Metadata preservation: EXIF=%v, GPS=%v", exifPreserved, gpsPreserved)

				// Log compression details
				if result.BeforeSize > 0 {
					compressionRatio := float64(result.BeforeSize-result.AfterSize) / float64(result.BeforeSize) * 100
					t.Logf("Compression: %.1f%% reduction", compressionRatio)
				}
			}
		})
	}
}

// checkMetadata checks if a JPEG file contains EXIF and GPS metadata
func checkMetadata(t *testing.T, filePath string) (hasEXIF bool, hasGPS bool) {
	// Use JPEG structure parsing for metadata detection
	return checkMetadataJPEGStructure(t, filePath)
}

// checkMetadataJPEGStructure attempts to detect EXIF/GPS using JPEG structure parsing
func checkMetadataJPEGStructure(t *testing.T, filePath string) (hasEXIF bool, hasGPS bool) {
	sl, err := parseJpeg(filePath)
	if err != nil {
		t.Logf("Failed to parse JPEG structure for %s: %v", filePath, err)
		return false, false
	}

	// Look for APP1 segments which typically contain EXIF data
	for _, segment := range sl.Segments() {
		if segment.MarkerId == 0xE1 { // APP1 marker
			// Check if this APP1 segment contains EXIF data
			if len(segment.Data) > 6 {
				exifHeader := string(segment.Data[:4])
				if exifHeader == "Exif" {
					hasEXIF = true
					t.Logf("Found EXIF data in %s (size: %d bytes)", filepath.Base(filePath), len(segment.Data))
					// Check for GPS data within EXIF
					if containsGPSData(segment.Data) {
						hasGPS = true
						t.Logf("Found GPS data in %s", filepath.Base(filePath))
					}
				}
			}
		}
	}

	return hasEXIF, hasGPS
}

// containsGPSData checks if EXIF data contains GPS information
func containsGPSData(exifData []byte) bool {
	// Look for GPS IFD tags within EXIF data
	// GPS tags typically include:
	// - GPS Version ID (0x0000)
	// - GPS Latitude Ref (0x0001)
	// - GPS Latitude (0x0002)
	// - GPS Longitude Ref (0x0003)
	// - GPS Longitude (0x0004)

	// Simple heuristic: look for common GPS tag patterns
	// This is a simplified detection - real implementation would need proper EXIF parsing
	gpsPatterns := []string{
		"GPS",      // GPS string literal
		"\x00\x01", // GPS Latitude Ref tag
		"\x00\x02", // GPS Latitude tag
		"\x00\x03", // GPS Longitude Ref tag
		"\x00\x04", // GPS Longitude tag
	}

	dataStr := string(exifData)
	for _, pattern := range gpsPatterns {
		if contains(dataStr, pattern) {
			return true
		}
	}

	return false
}
```

```go
package jpeg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evanoberholster/imagemeta"
	"github.com/ideamans/lightfile6-core/types"
)

func TestThumbnail(t *testing.T) {
	tempDir := t.TempDir()

	cases := []struct {
		name                    string
		file                    string
		expectedAbortType       string
		expectOriginalThumbnail bool // true if original should have embedded thumbnail
		expectFinalThumbnail    bool // true if final should have embedded thumbnail (should be false after optimization)
	}{
		{
			name:                    "Embedded thumbnail",
			file:                    "thumbnail_embedded.jpg",
			expectedAbortType:       types.AbortTypeNothing, // Should succeed
			expectOriginalThumbnail: true,                   // Should have thumbnail
			expectFinalThumbnail:    false,                  // Should be removed after optimization
		},
		{
			name:                    "No thumbnail",
			file:                    "thumbnail_none.jpg",
			expectedAbortType:       types.AbortTypeNothing, // Should succeed with PSNR check skipped
			expectOriginalThumbnail: false,                  // Should not have thumbnail
			expectFinalThumbnail:    false,                  // Should still not have thumbnail
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputPath := filepath.Join(".", "testdata", "variations", tc.file)
			outputPath := filepath.Join(tempDir, tc.file)

			// Check thumbnail in original file
			originalHasThumbnail, originalThumbnailSize := checkThumbnail(t, inputPath)
			t.Logf("Original file thumbnail: present=%v, size=%d bytes", originalHasThumbnail, originalThumbnailSize)

			// Verify our expectation matches reality for the original file
			if originalHasThumbnail != tc.expectOriginalThumbnail {
				t.Errorf("Expected original thumbnail presence %v, but got %v", tc.expectOriginalThumbnail, originalHasThumbnail)
			}

			option := &OptimizeJpegOption{}
			option.Quality = "medium"
			option.SkipPsnrAssertion = true

			result := Optimize(inputPath, outputPath, option)

			// Check optimization result
			if result.AbortType != tc.expectedAbortType {
				t.Errorf("Expected AbortType %q, got %q", tc.expectedAbortType, result.AbortType)
				if result.AbortDetail != nil {
					t.Logf("AbortDetail: %v", result.AbortDetail)
				}
			}

			// Log optimization results
			t.Logf("Optimization result: %d -> %d bytes", result.BeforeSize, result.AfterSize)
			t.Logf("Quality: %d, SSIM: %.6f, PSNR: %.2f", result.Quality, result.Ssim, result.FinalPsnr)
			t.Logf("AbortType: %s", result.AbortType)

			// Only check thumbnail removal if optimization was successful
			if result.AbortType == types.AbortTypeNothing {
				// Check that output file exists
				if _, err := os.Stat(outputPath); os.IsNotExist(err) {
					t.Error("Output file was not created")
					return
				}

				// Check thumbnail in optimized file
				optimizedHasThumbnail, optimizedThumbnailSize := checkThumbnail(t, outputPath)
				t.Logf("Optimized file thumbnail: present=%v, size=%d bytes", optimizedHasThumbnail, optimizedThumbnailSize)

				// Verify thumbnail removal
				if optimizedHasThumbnail != tc.expectFinalThumbnail {
					t.Errorf("Expected final thumbnail presence %v, got %v", tc.expectFinalThumbnail, optimizedHasThumbnail)
				}

				// Log thumbnail removal result
				if originalHasThumbnail && !optimizedHasThumbnail {
					t.Logf("Thumbnail successfully removed (saved %d bytes)", originalThumbnailSize)
				} else if !originalHasThumbnail && !optimizedHasThumbnail {
					t.Logf("No thumbnail to remove (as expected)")
				} else if originalHasThumbnail && optimizedHasThumbnail {
					t.Logf("Warning: Thumbnail was not removed")
				}
			}
		})
	}
}

// checkThumbnail checks if a JPEG file contains an embedded thumbnail and returns its size
func checkThumbnail(t *testing.T, filePath string) (hasThumbnail bool, thumbnailSize int) {
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	// Parse the image metadata using imagemeta
	em, err := imagemeta.Decode(file)
	if err != nil {
		t.Logf("Failed to decode image metadata for %s: %v", filePath, err)
		return false, 0
	}

	// Check if thumbnail is present
	// imagemeta should provide thumbnail information through EXIF data
	if em.ThumbnailOffset > 0 && em.ThumbnailLength > 0 {
		return true, int(em.ThumbnailLength)
	}

	// Alternative method: check for EXIF APP1 segments with thumbnail data
	// Reset file position and try JPEG structure parsing
	file.Seek(0, 0)

	// Try to detect thumbnail using JPEG structure
	hasThumbnailJpeg, sizeJpeg := checkThumbnailJPEGStructure(t, filePath)
	if hasThumbnailJpeg {
		return true, sizeJpeg
	}

	return false, 0
}

// checkThumbnailJPEGStructure attempts to detect thumbnail using JPEG structure parsing
func checkThumbnailJPEGStructure(t *testing.T, filePath string) (hasThumbnail bool, thumbnailSize int) {
	sl, err := parseJpeg(filePath)
	if err != nil {
		t.Logf("Failed to parse JPEG structure for %s: %v", filePath, err)
		return false, 0
	}

	// Look for APP1 segments which typically contain EXIF data with thumbnails
	for _, segment := range sl.Segments() {
		if segment.MarkerId == 0xE1 { // APP1 marker
			// Check if this APP1 segment contains EXIF data
			if len(segment.Data) > 6 {
				exifHeader := string(segment.Data[:4])
				if exifHeader == "Exif" {
					// This is an EXIF segment, check for thumbnail
					if containsThumbnail(segment.Data) {
						return true, len(segment.Data) // Approximate size
					}
				}
			}
		}
	}

	return false, 0
}

// containsThumbnail checks if EXIF data contains thumbnail information
func containsThumbnail(exifData []byte) bool {
	// Look for JPEG thumbnail markers within EXIF data
	// JPEG thumbnails in EXIF typically contain SOI (0xFFD8) and EOI (0xFFD9) markers
	for i := 0; i < len(exifData)-1; i++ {
		if exifData[i] == 0xFF && exifData[i+1] == 0xD8 {
			// Found SOI marker, look for corresponding EOI
			for j := i + 2; j < len(exifData)-1; j++ {
				if exifData[j] == 0xFF && exifData[j+1] == 0xD9 {
					// Found EOI marker, this indicates a JPEG thumbnail
					return true
				}
			}
		}
	}

	// Look for RGB thumbnail data (less common)
	// This is a simplified check - real implementation would need more sophisticated detection
	if len(exifData) > 1000 { // Arbitrary size threshold for potential thumbnail data
		return true
	}

	return false
}

```
