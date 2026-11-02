package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 依赖方向是硬规则：internal/* 是内核，只管协议与文件读写，对外只经 Logf 回调说话。
// 它不 import 主包（内核与界面解耦），也不自己打印（否则界面版与无界面版的日志会各写一套）。
// 这条约定靠人眼守不住，所以用 go list 与源码扫描机械校验；规则变了就改这里。

// bannedImports 是内部包不许 import 的包，值是给人看的原因。
var bannedImports = map[string]string{
	"pxeserv":  "内核与界面解耦，内部包不得 import 主包",
	"log":      "日志一律走 Logf 回调，不得自己打印",
	"log/slog": "日志一律走 Logf 回调，不得自己打印",
}

// printing 是绕开 Logf 直接往 stdout/stderr 写的调用。
var printing = regexp.MustCompile(`\b(?:fmt\.F?Print|print|println)\w*\(`)

// listed 是 go list 列出的一个内部包。
type listed struct {
	path    string
	dir     string
	imports []string
}

// internalPackages 列出 ./internal/... 的包及其 import。
func internalPackages(t *testing.T) []listed {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}}|{{.Dir}}|{{join .Imports ","}}`, "./internal/...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list 失败：%v\n%s", err, out)
	}
	var pkgs []listed
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.SplitN(line, "|", 3)
		if len(f) != 3 {
			t.Fatalf("go list 输出无法解析：%q", line)
		}
		p := listed{path: f[0], dir: f[1]}
		if f[2] != "" {
			p.imports = strings.Split(f[2], ",")
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("没有列出任何内部包：校验规则无从生效")
	}
	return pkgs
}

// TestInternalImports 校验内部包的 import：不碰主包，也不碰打印用的包。
func TestInternalImports(t *testing.T) {
	for _, p := range internalPackages(t) {
		for _, imp := range p.imports {
			if why, bad := bannedImport(imp); bad {
				t.Errorf("%s import 了 %q：%s", p.path, imp, why)
			}
		}
	}
}

// bannedImport 判断 imp 是否违规，返回原因。
func bannedImport(imp string) (string, bool) {
	if why, bad := bannedImports[imp]; bad {
		return why, true
	}
	// 内部包之间可以互相 import，但只能走 internal/：其它 pxeserv/... 都在主包这一侧。
	if strings.HasPrefix(imp, "pxeserv/") && !strings.HasPrefix(imp, "pxeserv/internal/") {
		return "内部包不得 import 主包一侧的 " + imp, true
	}
	return "", false
}

// TestBannedImport 校验规则本身：主包与打印用的包要拦，内部包与标准库要放行。
func TestBannedImport(t *testing.T) {
	cases := []struct {
		imp string
		bad bool
	}{
		{"pxeserv", true},
		{"pxeserv/debug", true},
		{"log", true},
		{"log/slog", true},
		{"pxeserv/internal/config", false},
		{"fmt", false},
		{"net", false},
	}
	for _, c := range cases {
		_, bad := bannedImport(c.imp)
		if bad != c.bad {
			t.Errorf("bannedImport(%q) = %v，想要 %v", c.imp, bad, c.bad)
		}
	}
}

// TestRootOnlyMain 校验布局：主包只许 main.go，其余代码进 internal/ 对应的包。
// 往根目录随手加文件是这种分层最常见的滲透方式，所以也机械校验。
func TestRootOnlyMain(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", `{{join .GoFiles ","}}`, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list 失败：%v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "main.go" {
		t.Errorf("主包文件是 %q，只应有 main.go：新代码请放进 internal/ 对应的包", got)
	}
}

// TestInternalNoPrint 扫描内部包的源码：不许有绕开 Logf 的打印调用。
// 按目录读全部 .go（含当前平台不参与编译的 _windows/_unix 分支），平台分支也守同一条规则。
func TestInternalNoPrint(t *testing.T) {
	for _, p := range internalPackages(t) {
		entries, err := os.ReadDir(p.dir)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", p.dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(p.dir, name)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("读取 %s 失败：%v", path, err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if m := printing.FindString(line); m != "" {
					t.Errorf("%s:%d 直接打印（%s）：日志请走 Logf 回调", path, i+1, strings.TrimSuffix(m, "("))
				}
			}
		}
	}
}
