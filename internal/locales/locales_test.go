package locales

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// catalog 是 catalogs 目录里一份 .json 解析后的样子：messages 是文案本身，
// meta 是 "@key" 声明（占位符类型等），二者都不含 "@@locale"。
type catalog struct {
	locale   string
	messages map[string]string
	meta     map[string]any
}

func loadCatalogs(t *testing.T) map[string]catalog {
	t.Helper()
	files, err := filepath.Glob("*.json")
	if err != nil {
		t.Fatalf("列出 catalog 失败：%v", err)
	}
	if len(files) == 0 {
		t.Fatal("没有找到任何 catalog（*.json）")
	}
	out := make(map[string]catalog, len(files))
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读 %s 失败：%v", name, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatalf("%s 不是合法 JSON：%v", name, err)
		}
		c := catalog{messages: map[string]string{}, meta: map[string]any{}}
		for k, v := range raw {
			switch {
			case k == "@@locale":
				s, _ := v.(string)
				c.locale = s
			case strings.HasPrefix(k, "@"):
				c.meta[strings.TrimPrefix(k, "@")] = v
			default:
				s, _ := v.(string)
				c.messages[k] = s
			}
		}
		out[strings.TrimSuffix(name, ".json")] = c
	}
	return out
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// placeholderRE 抓出 {name} 与 {name, plural, …} 里的 name；复数分支的正文
// （如 {=0 {no leases}} 里的 "no leases"）后跟的不是 } 或 , 所以不会被误认成占位符。
var placeholderRE = regexp.MustCompile(`\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*[},]`)

func placeholders(text string) map[string]bool {
	out := map[string]bool{}
	for _, m := range placeholderRE.FindAllStringSubmatch(text, -1) {
		out[m[1]] = true
	}
	return out
}

func TestCatalogs(t *testing.T) {
	sourceLocale := Catalog.Source()
	catalogs := loadCatalogs(t)
	if len(catalogs) < 2 {
		t.Fatalf("至少要有源语言与一种译文，实际 %d 份", len(catalogs))
	}
	source, ok := catalogs[sourceLocale]
	if !ok {
		t.Fatalf("缺少源语言 catalog：%s.json", sourceLocale)
	}
	for name, c := range catalogs {
		if c.locale == "" {
			t.Errorf("%s.json：缺少 @@locale", name)
		}
		if c.locale != name {
			t.Errorf("%s.json：@@locale=%q 与文件名不一致", name, c.locale)
		}
	}
	for name, c := range catalogs {
		if name == sourceLocale {
			continue
		}
		// 键集合必须与源语言一致，否则界面会有一半是空字符串。
		for _, k := range keys(source.messages) {
			if _, ok := c.messages[k]; !ok {
				t.Errorf("%s.json：缺少键 %q", name, k)
			}
		}
		for _, k := range keys(c.messages) {
			if _, ok := source.messages[k]; !ok {
				t.Errorf("%s.json：多了源语言没有的键 %q", name, k)
			}
		}
	}
	for name, c := range catalogs {
		for _, k := range keys(c.messages) {
			text := c.messages[k]
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s.json：%s 为空", name, k)
			}
			// 占位符必须与源语言同名同数量：漏一个参数，运行时就会暴露成
			// 渲染失败或错位的句子。
			want, got := placeholders(source.messages[k]), placeholders(text)
			for p := range want {
				if !got[p] {
					t.Errorf("%s.json：%s 缺少占位符 {%s}", name, k, p)
				}
			}
			for p := range got {
				if !want[p] {
					t.Errorf("%s.json：%s 多了占位符 {%s}", name, k, p)
				}
			}
			// 源语言里声明的占位符必须真的出现在文案里（反向的笔误检查）。
			if name == sourceLocale {
				if meta, ok := c.meta[k].(map[string]any); ok {
					if phs, ok := meta["placeholders"].(map[string]any); ok {
						for p := range phs {
							if !got[p] {
								t.Errorf("%s.json：%s 声明了占位符 {%s} 但文案里没有", name, k, p)
							}
						}
					}
				}
			}
		}
	}
}
