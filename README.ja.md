# go-exif-remove-thumbnail

[English README is here](README.md)

JPEG 画像の EXIF メタデータに埋め込まれたサムネイルを削除する Go 製のライブラリおよび CLI ツールです。

## 特徴

- JPEG 画像から EXIF サムネイルを削除
- CLI およびライブラリとして利用可能
- 外部依存なし（純粋な Go 実装）

## 使い方

### CLI

```sh
go run exifremovethumbnail.go -in input.jpg -out output.jpg
```

### ライブラリとして利用

#### ファイルベースの操作

```go
import "github.com/ideamans/go-exif-remove-thumbnail"

result, err := exifremovethumbnail.ExifRemoveThumbnail(inputPath, outputPath)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("サムネイル削除: %v, 削減サイズ: %dバイト\n", 
    result.HadThumbnail, result.ThumbnailSize)
```

#### メモリベースの操作

```go
import "github.com/ideamans/go-exif-remove-thumbnail"

// JPEGデータを読み込み
inputData, err := os.ReadFile("input.jpg")
if err != nil {
    log.Fatal(err)
}

// メモリ上でサムネイルを削除
outputData, result, err := exifremovethumbnail.ExifRemoveThumbnailBytes(inputData)
if err != nil {
    log.Fatal(err)
}

// 結果を書き込み
err = os.WriteFile("output.jpg", outputData, 0644)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("サムネイル削除: %v, 削減サイズ: %dバイト\n", 
    result.HadThumbnail, result.ThumbnailSize)
```

## テスト

```sh
go test
```

## テスト画像

テスト用画像は `testdata/` ディレクトリにあります。

## 戻り値の構造体

ライブラリ関数 `ExifRemoveThumbnail` の戻り値は `ExifRemoveThumbnailResult` 構造体です。

```go
// サムネイル削除処理の結果
 type ExifRemoveThumbnailResult struct {
     HadThumbnail  bool   // 元画像にサムネイルが存在したか
     BeforeSize    int64  // 元画像のファイルサイズ
     AfterSize     int64  // 出力画像のファイルサイズ
     ThumbnailSize int64  // 削除されたサムネイルのサイズ
 }
```

- `HadThumbnail`: 元画像にサムネイルが存在した場合は true
- `BeforeSize`: 入力画像のバイトサイズ
- `AfterSize`: 出力画像のバイトサイズ
- `ThumbnailSize`: 削除されたサムネイルのバイトサイズ（サムネイルがなければ 0）

## ライセンス

MIT License
