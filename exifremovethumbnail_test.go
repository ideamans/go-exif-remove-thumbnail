package exifremovethumbnail_test

import (
	"bytes"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/rwcarlsen/goexif/exif"
	_ "github.com/rwcarlsen/goexif/mknote"
	"github.com/stretchr/testify/require"

	exifremovethumbnail "github.com/ideamans/go-exif-remove-thumbnail"
)

func TestExifRemoveThumbnail(t *testing.T) {
	dir := "testdata"
	tests := []struct {
		name         string
		file         string
		hasThumbnail bool
		hasGPS       bool
		hasExif      bool
	}{
		{"サムネイルあり", filepath.Join(dir, "thumbnail_embedded.jpg"), true, false, true},
		{"サムネイルなし", filepath.Join(dir, "thumbnail_none.jpg"), false, false, false},
		{"基本EXIF", filepath.Join(dir, "metadata_basic_exif.jpg"), false, false, true},
		{"完全EXIF", filepath.Join(dir, "metadata_full_exif.jpg"), false, false, true},
		{"GPS EXIF", filepath.Join(dir, "metadata_gps.jpg"), false, true, true},
		{"EXIFなし", filepath.Join(dir, "metadata_none.jpg"), false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.file + ".out.jpg"
			os.Remove(out)
			res, err := exifremovethumbnail.ExifRemoveThumbnail(tt.file, out)
			require.NoError(t, err)
			defer os.Remove(out)
			if tt.hasThumbnail {
				require.True(t, res.HadThumbnail, "should detect thumbnail")
				require.Greater(t, res.ThumbnailSize, int64(0))
				require.Greater(t, res.BeforeSize, res.AfterSize)
				// サムネイルがあった場合はAfterSizeがBeforeSizeより小さいこと
				require.Less(t, res.AfterSize, res.BeforeSize, "サムネイル削除後はファイルサイズが小さくなるべき")
			} else {
				require.False(t, res.HadThumbnail, "should not detect thumbnail")
			}

			// 1. 処理前の画像が期待するメタデータを持ち合わせていること
			inData, err := os.ReadFile(tt.file)
			require.NoError(t, err)
			inExif, _ := exif.Decode(bytes.NewReader(inData))
			if tt.hasExif {
				// Exifがなければスキップ（goexifはAPP1が無いとnilを返す）
				require.NotNil(t, inExif, "入力画像はExifを持つべき (ただしAPP1がなければnil)")
			} else {
				// Exifが無い場合はnilでOK
				require.Nil(t, inExif, "入力画像はExifを持たないべき")
			}
			if tt.hasGPS && inExif != nil {
				_, err := inExif.Get(exif.GPSInfoIFDPointer)
				require.NoError(t, err, "入力画像はGPSを持つべき")
			}

			// 2. 処理後にサムネイルが削除されていること
			outData, err := os.ReadFile(out)
			require.NoError(t, err)
			outExif, _ := exif.Decode(bytes.NewReader(outData))
			if outExif != nil {
				// サムネイルタグが消えていること（IFD1が消えていること）
				thumb, err := outExif.JpegThumbnail()
				require.Error(t, err, "出力画像はサムネイルを持たないべき")
				require.Nil(t, thumb, "サムネイルバイト列もnilであるべき")
			}

			// 3. サムネイル以外のメタデータが保持されていること
			if tt.hasExif {
				require.NotNil(t, outExif, "出力画像もExifを持つべき")
			}
			if tt.hasGPS && outExif != nil {
				_, err := outExif.Get(exif.GPSInfoIFDPointer)
				require.NoError(t, err, "出力画像もGPSを持つべき")
			}

			// 4. JPEGファイルとしてデコード可能であること
			_, err = jpeg.Decode(bytes.NewReader(outData))
			require.NoError(t, err, "JPEGデコード可能であるべき")

			// 5. ファイルサイズの検証
			require.Equal(t, res.BeforeSize, int64(len(inData)), "BeforeSizeは入力ファイルサイズと一致するべき")
			require.Equal(t, res.AfterSize, int64(len(outData)), "AfterSizeは出力ファイルサイズと一致するべき")
		})
	}
}

func TestFormatError(t *testing.T) {
	// PNGファイルをJPEGとして処理
	file := filepath.Join("testdata", "actual_png.jpg")
	out := file + ".out.jpg"
	defer os.Remove(out)
	_, err := exifremovethumbnail.ExifRemoveThumbnail(file, out)
	require.Error(t, err)
	_, ok := err.(*exifremovethumbnail.FormatError)
	require.True(t, ok, "FormatErrorであるべき")

	// 存在しないファイルのテスト
	file = filepath.Join("testdata", "not_found.jpg")
	out = file + ".out.jpg"
	_, err = exifremovethumbnail.ExifRemoveThumbnail(file, out)
	require.Error(t, err)
}
