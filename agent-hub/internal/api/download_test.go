package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub/internal/api"
	"github.com/superfive666/agent-hub/agent-hub/internal/config"
)

// newAPKServer 起一个**不连库**的服务。
//
// 下载端点完全不碰 store，所以这里刻意不用 newServer —— 那个要 TEST_DATABASE_URL，
// 没配就整包跳过。而「不登录也能下载」这条需求是这个端点存在的理由，
// 它必须在任何一台机器上都跑得到，不能因为本地没起 postgres 就被静默跳过。
func newAPKServer(t *testing.T, apkPath, version string) *httptest.Server {
	t.Helper()
	cfg := config.Config{
		DatabaseURL: "unused", Timezone: "UTC", AuthMode: config.AuthPassword,
		AdminUsername: "superfive", AdminPasswordHash: testPasswordHash,
		SessionSecret: "test-secret-0123456789", LongPollMax: 30 * time.Second,
		AndroidAPKPath: apkPath, AndroidAPKVersion: version,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("测试配置本身不合法: %v", err)
	}
	srv := httptest.NewServer(api.New(nil, cfg, nil).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// writeAPK 造一份假的安装包。内容不重要，重要的是它是个真实存在、有大小的文件。
func writeAPK(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app-release.apk")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// 需求：无论登录与否都能下载这个 apk，路径是 /download。
//
// 这是这个端点存在的**全部理由**。装 app 的那一刻用户手上还没有会话，
// 而他很可能正是想在手机上登录才来装的 —— 一旦哪天有人手滑给它套上
// requireAdmin，症状是「新用户永远装不上」，而登录着的开发者自己测不出来。
func TestAPKDownloadIsPublic(t *testing.T) {
	srv := newAPKServer(t, writeAPK(t, "PK\x03\x04fake-apk"), "1.2.0")

	resp, body := getWith(t, srv.URL+"/download", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("没带任何凭证也应当 200，实得 %d", resp.StatusCode)
	}
	if got := string(body); got != "PK\x03\x04fake-apk" {
		t.Errorf("下发的内容不是那份文件: %q", got)
	}
}

// 需求：下下来的东西必须是「一个能装的 apk」。
//
// Content-Type 决定了 Android 浏览器下完之后弹不弹「安装」。给成
// application/octet-stream 的话文件确实下来了，但它躺在下载目录里点了没反应 ——
// 而这个失败看起来完全不像是服务端的问题。
func TestAPKDownloadHeaders(t *testing.T) {
	srv := newAPKServer(t, writeAPK(t, "fake"), "1.2.0")

	resp, _ := getWith(t, srv.URL+"/download", "")
	if got := resp.Header.Get("Content-Type"); got != "application/vnd.android.package-archive" {
		t.Errorf("Content-Type = %q，不是 apk 的 MIME —— 手机上下完不会弹安装", got)
	}
	// 文件名要带版本：用户的下载目录里会同时躺着好几个版本，都叫 app.apk 的话
	// 没人分得清哪个是新的。
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, `filename="agent-hub-1.2.0.apk"`) {
		t.Errorf("Content-Disposition = %q，want 带版本号的文件名", got)
	}
	// 发版是原地替换同一个路径，中间层拿旧包顶新包的话，用户装到的永远是上一版。
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "must-revalidate") {
		t.Errorf("Cache-Control = %q，缺 must-revalidate —— 代理会拿旧包顶新包", got)
	}
}

// 需求：这台 hub 拿不出包时，要说得清楚是**部署**的问题，不是路由的问题。
//
// 404 的意思是「没有这个端点」，会把运维引去查代码；真实情况是端点在、
// 只是 CI 还没把产物放上来。两者要查的地方完全不同。
func TestAPKDownloadUnavailableIs503(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "没有配置 ANDROID_APK_PATH", path: ""},
		{name: "配了路径但产物还没放上来", path: filepath.Join(t.TempDir(), "never-built.apk")},
		{name: "路径指到了一个目录", path: t.TempDir()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newAPKServer(t, tt.path, "1.2.0")

			resp, body := getWith(t, srv.URL+"/download", "")
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", resp.StatusCode)
			}
			var e api.Error
			if err := json.Unmarshal(body, &e); err != nil {
				t.Fatalf("响应不是结构化错误: %v (%s)", err, body)
			}
			if e.Code != "apk_unavailable" {
				t.Errorf("code = %q, want apk_unavailable", e.Code)
			}
			// 可重试且给了 Retry-After：产物是事后放上去的，等一会儿真的可能就有了。
			// 不给的话调用方只能自己猜间隔。
			if !e.Retryable || resp.Header.Get("Retry-After") == "" {
				t.Error("拿不出包是暂时状态，必须可重试并给出 Retry-After")
			}
			// 不能把服务器上的文件布局漏出去。
			if strings.Contains(e.Message, tt.path) && tt.path != "" {
				t.Error("错误信息里带了真实路径 —— 对外泄露了文件布局")
			}
		})
	}
}

// 需求：手机上下十几 MB 的包，断一次不能从头再来。
//
// 断点续传不是锦上添花：地铁里、电梯里断流是常态，没有 Range 支持的话
// 用户会卡在 90% 反复重来，而服务端日志上看不出任何异常。
func TestAPKDownloadSupportsRange(t *testing.T) {
	srv := newAPKServer(t, writeAPK(t, "0123456789"), "1.2.0")

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/download", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=4-7")
	resp, body := do(t, req)

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206 —— 不支持 Range 就没有断点续传", resp.StatusCode)
	}
	if got := string(body); got != "4567" {
		t.Errorf("body = %q, want %q", got, "4567")
	}
}

// 需求：改过反向代理的和没改过的部署都要能下到。
//
// /download 不在 /api/ 下，而 docs/08-deployment.md §5 的代理只转发 /api/* 和 /healthz。
// 少了这个同义词，老配置下 /download 会被静态站接走，用户下到一个改名叫 .apk 的
// index.html —— 安装器只说「解析包时出现问题」，没有任何线索指向代理。
func TestAPKDownloadAlsoUnderAPIPrefix(t *testing.T) {
	srv := newAPKServer(t, writeAPK(t, "fake"), "1.2.0")

	for _, p := range []string{"/download", "/api/download"} {
		resp, body := getWith(t, srv.URL+p, "")
		if resp.StatusCode != http.StatusOK || string(body) != "fake" {
			t.Errorf("%s: status=%d body=%q，两条路径必须是同一个处理器", p, resp.StatusCode, body)
		}
	}
}

// 需求：版本号来自环境变量，不能让它把响应头拆开。
//
// ANDROID_APK_VERSION 是部署时填的，最终会进 Content-Disposition。带上引号或
// 换行就能注入一个额外的响应头，所以要过滤而不是信任。
func TestAPKFilenameSanitizesVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "正常版本号原样保留", version: "1.2.0-rc1", want: `filename="agent-hub-1.2.0-rc1.apk"`},
		{name: "留空时退回不带版本的默认名", version: "", want: `filename="agent-hub.apk"`},
		{name: "带引号和换行的注入尝试被剥干净", version: "1.0\"\r\nX-Evil: 1", want: `filename="agent-hub-1.0X-Evil1.apk"`},
		{name: "路径分隔符不能出现在文件名里", version: "../../etc/passwd", want: `filename="agent-hub-....etcpasswd.apk"`},
		{name: "全是点和横杠时退回默认名", version: "..--", want: `filename="agent-hub.apk"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newAPKServer(t, writeAPK(t, "fake"), tt.version)

			resp, _ := getWith(t, srv.URL+"/download", "")
			got := resp.Header.Get("Content-Disposition")
			if got != "attachment; "+tt.want {
				t.Errorf("Content-Disposition = %q, want 含 %q", got, tt.want)
			}
			if resp.Header.Get("X-Evil") != "" {
				t.Error("版本号把一个额外的响应头注进来了")
			}
		})
	}
}

// 需求：控制台的下载入口要在点之前就知道会得到什么。
//
// 没有这个端点，页面只能画一个不知道会发生什么的按钮 —— 没包时用户点下去
// 拿到一段 JSON 错误。有了它，没包时按钮就是禁用加说明。
func TestAPKMeta(t *testing.T) {
	t.Run("有包时给出版本与体积", func(t *testing.T) {
		srv := newAPKServer(t, writeAPK(t, "0123456789"), "1.2.0")

		resp, body := getWith(t, srv.URL+"/download/meta", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var m struct {
			Available bool   `json:"available"`
			Version   string `json:"version"`
			Filename  string `json:"filename"`
			SizeBytes int64  `json:"sizeBytes"`
			UpdatedAt string `json:"updatedAt"`
		}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("响应不是 JSON: %v (%s)", err, body)
		}
		if !m.Available || m.Version != "1.2.0" || m.SizeBytes != 10 {
			t.Errorf("meta = %+v，want available=true version=1.2.0 size=10", m)
		}
		if m.Filename != "agent-hub-1.2.0.apk" {
			t.Errorf("filename = %q", m.Filename)
		}
		if _, err := time.Parse(time.RFC3339, m.UpdatedAt); err != nil {
			t.Errorf("updatedAt = %q，不是 RFC3339", m.UpdatedAt)
		}
	})

	// 没包时**不是错误**，是一个确定的答案：available=false。
	// 回 503 的话控制台得把「暂时没包」和「hub 挂了」当成同一件事处理。
	t.Run("没包时是 200 加 available=false，不是错误", func(t *testing.T) {
		srv := newAPKServer(t, "", "")

		resp, body := getWith(t, srv.URL+"/download/meta", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var m struct {
			Available bool `json:"available"`
		}
		if err := json.Unmarshal(body, &m); err != nil || m.Available {
			t.Errorf("body = %s，want available=false", body)
		}
	})

	t.Run("同样有 /api 前缀的同义词", func(t *testing.T) {
		srv := newAPKServer(t, writeAPK(t, "fake"), "1.0")

		resp, _ := getWith(t, srv.URL+"/api/download/meta", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})
}
