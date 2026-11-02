package httpd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 起一个只带 index 的 httptest，返回它的 URL。
func newTestServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := New(root, 0, nil)
	ts := httptest.NewServer(s.srv.Handler)
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// 根目录要给人看：列出共享的启动文件。
func TestIndexListsFiles(t *testing.T) {
	ts := newTestServer(t, map[string]string{
		"menu.ipxe":         "#!ipxe\n",
		"wimboot":           "x",
		"wim-boot/boot.sdi": "y",
	})
	code, body := get(t, ts.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("/ 状态码 = %d，应为 200", code)
	}
	for _, want := range []string{"PXE 启动服务器", "/menu.ipxe", "/wimboot", "wim-boot/"} {
		if !strings.Contains(body, want) {
			t.Fatalf("/ 首页缺少 %q\n%s", want, body)
		}
	}
}

// 文件照常能取，且子目录仍然不列举（防止变成可枚举的文件服务器）。
func TestFileServedButSubdirNotListed(t *testing.T) {
	ts := newTestServer(t, map[string]string{"menu.ipxe": "#!ipxe\n", "wim-boot/boot.sdi": "y"})
	if code, body := get(t, ts.URL+"/menu.ipxe"); code != http.StatusOK || !strings.Contains(body, "#!ipxe") {
		t.Fatalf("/menu.ipxe = %d %q", code, body)
	}
	if code, _ := get(t, ts.URL+"/wim-boot/"); code != http.StatusNotFound {
		t.Fatalf("/wim-boot/ 状态码 = %d，应为 404", code)
	}
}
