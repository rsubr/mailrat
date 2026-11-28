package ui

import (
	"image/color"
	"path/filepath"
	"strings"
)

// attachKind is the Gmail-style badge shown on an attachment tile.
type attachKind struct {
	Label string
	Color color.NRGBA
}

var (
	kindPDF     = attachKind{"PDF", color.NRGBA{0xd9, 0x3a, 0x2f, 0xff}}
	kindWord    = attachKind{"DOC", color.NRGBA{0x2a, 0x5a, 0xd6, 0xff}}
	kindExcel   = attachKind{"XLS", color.NRGBA{0x1e, 0x8e, 0x4a, 0xff}}
	kindPPT     = attachKind{"PPT", color.NRGBA{0xe0, 0x6a, 0x1b, 0xff}}
	kindText    = attachKind{"TXT", color.NRGBA{0x75, 0x7b, 0x85, 0xff}}
	kindImage   = attachKind{"IMG", color.NRGBA{0x8e, 0x44, 0xad, 0xff}}
	kindArchive = attachKind{"ZIP", color.NRGBA{0xb0, 0x8a, 0x1a, 0xff}}
	kindAudio   = attachKind{"AUD", color.NRGBA{0x0e, 0x8f, 0x8f, 0xff}}
	kindVideo   = attachKind{"VID", color.NRGBA{0xc2, 0x3a, 0x7a, 0xff}}
	kindCode    = attachKind{"</>", color.NRGBA{0x3d, 0x4f, 0x6b, 0xff}}
	kindFile    = attachKind{"FILE", color.NRGBA{0x5f, 0x6b, 0x7a, 0xff}}
)

var kindByExt = map[string]attachKind{
	"pdf": kindPDF, "doc": kindWord, "docx": kindWord, "odt": kindWord, "rtf": kindWord,
	"xls": kindExcel, "xlsx": kindExcel, "csv": kindExcel, "ods": kindExcel,
	"ppt": kindPPT, "pptx": kindPPT, "odp": kindPPT, "key": kindPPT,
	"txt": kindText, "text": kindText, "log": kindText, "md": kindText,
	"png": kindImage, "jpg": kindImage, "jpeg": kindImage, "gif": kindImage, "webp": kindImage,
	"bmp": kindImage, "svg": kindImage, "heic": kindImage, "tif": kindImage, "tiff": kindImage,
	"zip": kindArchive, "rar": kindArchive, "7z": kindArchive, "tar": kindArchive, "gz": kindArchive, "tgz": kindArchive,
	"mp3": kindAudio, "wav": kindAudio, "m4a": kindAudio, "flac": kindAudio, "ogg": kindAudio,
	"mp4": kindVideo, "mov": kindVideo, "avi": kindVideo, "mkv": kindVideo, "webm": kindVideo,
	"go": kindCode, "js": kindCode, "ts": kindCode, "py": kindCode, "java": kindCode, "c": kindCode,
	"cpp": kindCode, "h": kindCode, "html": kindCode, "css": kindCode, "json": kindCode, "xml": kindCode, "sh": kindCode,
}

// attachmentKind maps a file name (extension first) or MIME type to its badge.
func attachmentKind(name, mime string) attachKind {
	if k, ok := kindByExt[strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))]; ok {
		return k
	}
	mime = strings.ToLower(mime)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return kindImage
	case strings.HasPrefix(mime, "audio/"):
		return kindAudio
	case strings.HasPrefix(mime, "video/"):
		return kindVideo
	case mime == "application/pdf":
		return kindPDF
	case strings.HasPrefix(mime, "text/"):
		return kindText
	case strings.Contains(mime, "zip") || strings.Contains(mime, "compressed"):
		return kindArchive
	case strings.Contains(mime, "word"):
		return kindWord
	case strings.Contains(mime, "excel") || strings.Contains(mime, "spreadsheet"):
		return kindExcel
	case strings.Contains(mime, "powerpoint") || strings.Contains(mime, "presentation"):
		return kindPPT
	}
	return kindFile
}
