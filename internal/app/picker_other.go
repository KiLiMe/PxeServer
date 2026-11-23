//go:build !linux

package app

// pickFolderExternal 在非 Linux 平台没有「桌面选择器」这回事：
// 一律回落 mygo 的原生对话框（Windows 的 IFileOpenDialog）。
func pickFolderExternal(string, string) (string, bool) { return "", false }
