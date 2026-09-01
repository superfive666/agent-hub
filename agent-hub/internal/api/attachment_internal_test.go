package api

import "testing"

// 这几个函数在包内测，不走 HTTP。
//
// 理由是**端到端测不出层次**：safeFilename 已经把控制字符去掉了，所以从
// HTTP 那一头打进来的恶意文件名，走到 contentDisposition 时已经是干净的 ——
// 端到端用例照样绿，哪怕 contentDisposition 换成天真的字符串拼接。
// 这一点是实测出来的，不是推的。
//
// 而 contentDisposition 是**最后一道**：safeFilename 将来一放宽，
// 它就是唯一挡着响应头注入的东西。所以它得有自己的用例，直接喂恶意输入。

func TestContentDispositionNeverEmitsHeaderDelimiters(t *testing.T) {
	hostile := []string{
		"a\r\nX-Injected: yes",
		"a\nX-Injected: yes",
		"a\rX-Injected: yes",
		`quote".txt`,
		`back\slash.txt`,
		"分号;和逗号,.txt",
		"\x00nul.txt",
		"tab\there.txt",
	}
	for _, name := range hostile {
		got := contentDisposition(name)
		for _, bad := range []string{"\r", "\n", "\x00"} {
			if containsByte(got, bad[0]) {
				t.Errorf("contentDisposition(%q) 里出现了分隔符 %q: %q", name, bad, got)
			}
		}
		// filename="..." 那一份里不许有能提前闭合它的引号或反斜杠
		if q := quotedPart(got); containsByte(q, '"') || containsByte(q, '\\') {
			t.Errorf("contentDisposition(%q) 的 filename= 部分能被提前闭合: %q", name, got)
		}
		if !hasPrefix(got, "attachment;") {
			t.Errorf("contentDisposition(%q) 必须以 attachment; 开头: %q", name, got)
		}
	}
}

// 中文名要能完整传达 —— 走 RFC 5987 的 filename*，百分号编码。
// 没有它的话所有中文文件名下下来都叫 ____.txt，功能上是可用的，
// 但人拿到一堆下划线是分不清哪个是哪个的。
func TestContentDispositionKeepsNonASCIINamesViaRFC5987(t *testing.T) {
	got := contentDisposition("季度报告.pdf")
	if !contains(got, "filename*=UTF-8''") {
		t.Fatalf("非 ASCII 文件名要走 filename*: %q", got)
	}
	// %E5%AD%A3 = “季”
	if !contains(got, "%E5%AD%A3") {
		t.Errorf("filename* 应该是 UTF-8 的百分号编码: %q", got)
	}
	// 老客户端看的那一份要退化成一个安全的 ASCII 名，而不是空
	if !contains(got, `filename="`) {
		t.Errorf("还要留一份 ASCII 的 filename= 给不认识 filename* 的客户端: %q", got)
	}
}

// 老客户端只认 filename= 那一份。**扩展名必须保下来** ——
// 「构建报告.txt」里一个 ASCII 字母都没有，逐字符过滤完再一修就只剩 "txt"，
// 存下来是个没有扩展名的文件，双击打不开。
func TestASCIIFallbackKeepsTheExtension(t *testing.T) {
	tests := []struct {
		give, want string
	}{
		// 主干里一个字母数字都不剩 → 退回 attachment，但扩展名留着，
		// 老客户端存下来的至少还是个双击打得开的 .txt
		{"构建报告.txt", "attachment.txt"},
		// 主干里还剩 v2 就用剩下的（四个汉字 + 一个空格 = 五个下划线）
		{"季度报告 v2.pdf", "_____v2.pdf"},
		{"report.txt", "report.txt"},
		{"。。。", "attachment"}, // 连扩展名都没有，只能退到裸兜底
		{"", "attachment"},
		// 开头那个点不算扩展名分隔符，隐藏文件保留原名
		{".gitignore", ".gitignore"},
		{"a.tar.gz", "a.tar.gz"},
	}
	for _, tt := range tests {
		if got := asciiFallbackName(tt.give); got != tt.want {
			t.Errorf("asciiFallbackName(%q) = %q, want %q", tt.give, got, tt.want)
		}
	}
}

func TestContentDispositionFallsBackWhenNothingSurvives(t *testing.T) {
	// 全是会被过滤掉的字符 —— 不能拼出 filename=""
	got := contentDisposition("。。。")
	if contains(got, `filename=""`) {
		t.Errorf("ASCII 那一份不能是空的: %q", got)
	}
	if !contains(got, "attachment") {
		t.Errorf("兜底名字里该有点内容: %q", got)
	}
}

func TestSafeFilenameStripsPathsAndInvisibles(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{"去掉 POSIX 路径", "../../etc/passwd", "passwd"},
		{"去掉 Windows 路径", `C:\Users\x\报告.txt`, "报告.txt"},
		{"去掉换行", "a\r\nb.txt", "ab.txt"},
		// 双向控制符是隐形的：它能让 "报告.txt" 在界面上看起来是 "报告.exe"
		{"去掉双向控制符", "报告\u202egpj.txt", "报告gpj.txt"},
		{"全被过滤掉时给个兜底名", "///", "attachment"},
		{"只剩点也给兜底名", "..", "attachment"},
		{"普通中文名原样留着", "季度报告 v2.pdf", "季度报告 v2.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeFilename(tt.give); got != tt.want {
				t.Errorf("safeFilename(%q) = %q, want %q", tt.give, got, tt.want)
			}
		})
	}
}

func TestSafeFilenameTruncatesByRuneNotByte(t *testing.T) {
	// 200 个中文字符。按字节截会把最后一个字切成半个，界面上是个乱码方块。
	long := ""
	for i := 0; i < 200; i++ {
		long += "报"
	}
	got := safeFilename(long)
	if r := []rune(got); len(r) != 120 {
		t.Errorf("应该截到 120 个字符，得到 %d", len(r))
	}
	for _, r := range got {
		if r != '报' {
			t.Fatalf("截断把字切碎了: %q", got)
		}
	}
}

// 类型协商是这个功能的安全边界之一：**上传方声明的类型不直接回显**。
func TestNormalizeContentTypeOnlyEchoesTheAllowlist(t *testing.T) {
	tests := []struct {
		name           string
		declared, file string
		want           string
	}{
		{"png 原样留着", "image/png", "a.png", "image/png"},
		{"带参数的也认", "image/png; name=x", "a.png", "image/png"},
		{"大小写不敏感", "IMAGE/PNG", "a.png", "image/png"},
		{"文本强制 utf-8", "text/plain", "a.txt", "text/plain; charset=utf-8"},
		{"声明的 charset 不采信", "text/plain; charset=gbk", "a.txt", "text/plain; charset=utf-8"},
		{"pdf 放行", "application/pdf", "a.pdf", "application/pdf"},

		// —— 下面这些一个都不许回显 ——
		{"html 降级", "text/html", "a.html", "application/octet-stream"},
		{"xhtml 降级", "application/xhtml+xml", "a.xhtml", "application/octet-stream"},
		{"svg 降级", "image/svg+xml", "a.svg", "application/octet-stream"},
		{"js 降级", "application/javascript", "a.js", "application/octet-stream"},
		{"没听说过的降级", "application/x-whatever", "a.bin", "application/octet-stream"},
		{"畸形的声明降级", "not a media type", "a.bin", "application/octet-stream"},

		// 没带声明时按扩展名兜底 —— 很多 CLI 上传工具就是不带
		{"没声明按扩展名认 png", "", "shot.PNG", "image/png"},
		{"没声明也不给 html 开口子", "", "page.html", "application/octet-stream"},
		{"没声明也没扩展名", "", "README", "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeContentType(tt.declared, tt.file); got != tt.want {
				t.Errorf("normalizeContentType(%q, %q) = %q, want %q",
					tt.declared, tt.file, got, tt.want)
			}
		})
	}
}

func TestHumanBytesReadsLikeALimit(t *testing.T) {
	tests := []struct {
		give int64
		want string
	}{
		{25 << 20, "25 MiB"},
		{512 << 10, "512 KiB"},
		{999, "999 B"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.give); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.give, got, tt.want)
		}
	}
}

// —— 小工具，避免为了三个判断把 strings 引进来 ——

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

// quotedPart 取 filename="…" 里引号之间那一段（到第一个后引号为止）。
func quotedPart(s string) string {
	const marker = `filename="`
	start := -1
	for i := 0; i+len(marker) <= len(s); i++ {
		if s[i:i+len(marker)] == marker {
			start = i + len(marker)
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start; i < len(s); i++ {
		if s[i] == '"' {
			return s[start:i]
		}
	}
	return s[start:]
}
