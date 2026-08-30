package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"
)

// apkContentType 是 Android 包的标准 MIME。
//
// 不能用 application/octet-stream：部分 Android 浏览器（以及国产 ROM 自带的下载器）
// 按 Content-Type 决定下完之后要不要弹「安装」，给成 octet-stream 的话文件是下来了，
// 但用户拿到的是一个躺在下载目录里、点了没反应的东西。
const apkContentType = "application/vnd.android.package-archive"

// ErrAPKUnavailable 是「这台 hub 上没有可下载的 APK」。
//
// **必须是一个说得清楚的 503，不能是 404。** 404 的意思是「没有这个端点」，
// 而真实情况是端点在、只是构建产物还没放上来 —— 两者的处理方式完全不同：
// 前者该去查代码，后者该去查部署。返回 404 等于把运维往错误的方向指。
//
// Retryable 为 true：产物是 CI 事后放上去的，等一会儿再来确实可能就有了。
var ErrAPKUnavailable = Error{
	Code:       "apk_unavailable",
	Message:    "这台 hub 还没有可下载的 Android 包：需要配置 ANDROID_APK_PATH 并把构建产物放到该路径",
	Retryable:  true,
	RetryAfter: 300,
}

// handleAPKDownload 把 Android 客户端的安装包交出去。
//
// **公开，不挂 requireAdmin。** 这是刻意的：装 app 的那一刻用户手上还没有会话，
// 而且很可能就是想在手机上登录才来装的 —— 把下载挂在登录后面就成了
// 「要先登录才能拿到用来登录的东西」。APK 本身不含任何实例数据，
// 它是一个空壳客户端，hub 地址和凭据都是用户装完自己填的。
//
// 走 http.ServeContent 而不是自己 io.Copy，为的是**断点续传**：
// 手机上下十几 MB 的包，地铁里断一次就得从头再来。ServeContent 免费给到
// Range、If-Modified-Since 和 Content-Length，这三样都值得要。
func (s *Server) handleAPKDownload(w http.ResponseWriter, r *http.Request) {
	f, info, err := s.openAPK()
	if err != nil {
		writeErr(w, ErrAPKUnavailable)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", apkContentType)
	// 文件名带版本号 —— 用户的下载目录里会同时躺着好几个版本，都叫 app.apk
	// 的话没人分得清哪个是新的。
	w.Header().Set("Content-Disposition", `attachment; filename="`+s.apkFilename()+`"`)
	// 发版是**原地替换同一个路径**，所以不能让中间层拿旧包顶新包。
	// no-store 又太狠（断点续传要靠再次请求同一份内容），must-revalidate 是这里的正解。
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// name 传空串：ServeContent 会拿它去猜 Content-Type，而我们上面已经定死了。
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// handleAPKMeta 告诉调用方「现在有没有包、是哪个版本、多大」。
//
// 控制台的下载入口要靠它：没有这个端点，页面只能画一个不知道会发生什么的按钮，
// 用户点下去可能拿到一段 JSON 错误。有了它，没包时按钮就是禁用+说明，
// 有包时能直接把版本和体积写在按钮边上 —— 这是「点之前就知道会得到什么」。
func (s *Server) handleAPKMeta(w http.ResponseWriter, r *http.Request) {
	f, info, err := s.openAPK()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	defer f.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"version":   s.cfg.AndroidAPKVersion,
		"filename":  s.apkFilename(),
		"sizeBytes": info.Size(),
		"updatedAt": info.ModTime().UTC().Format(time.RFC3339),
	})
}

// openAPK 打开配置里指向的那份产物。
//
// 「没配置」「文件不在」「指到了一个目录」在调用方看来是同一件事 ——
// 这台 hub 现在拿不出包 —— 所以合并成一个错误，细节只进日志。
// 把它们区分成不同的 HTTP 响应等于对外泄露服务器上的文件布局。
func (s *Server) openAPK() (*os.File, fs.FileInfo, error) {
	p := strings.TrimSpace(s.cfg.AndroidAPKPath)
	if p == "" {
		return nil, nil, errors.New("未配置 ANDROID_APK_PATH")
	}
	f, err := os.Open(p)
	if err != nil {
		// 未配置是常态（大多数部署不发 app），文件缺失才值得记一笔 ——
		// 那说明有人配了路径但 CI 没把产物放上去。
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Error("打开 APK 失败", "path", p, "err", err)
		}
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		f.Close()
		s.log.Error("ANDROID_APK_PATH 不是一个可读的文件", "path", p)
		return nil, nil, errors.New("不是可读文件")
	}
	return f, info, nil
}

// apkFilename 是下载下来的文件名。
//
// 只取配置版本号里「文件名安全」的那部分：这个值来自环境变量，最终会进
// Content-Disposition 头。带上引号或换行就能把这个头拆开，所以必须过滤，
// 而不是信任它。
func (s *Server) apkFilename() string {
	v := sanitizeVersion(s.cfg.AndroidAPKVersion)
	if v == "" {
		return "agent-hub.apk"
	}
	return "agent-hub-" + v + ".apk"
}

func sanitizeVersion(v string) string {
	var b strings.Builder
	alnum := false
	for _, r := range strings.TrimSpace(v) {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			alnum = true
			b.WriteRune(r)
		case r == '.', r == '-', r == '_', r == '+':
			b.WriteRune(r)
		}
	}
	// 至少要有一个字母或数字。全是点和横杠的话（".."、"---"）拼出来的是
	// agent-hub-...apk 这种看不出是什么的文件名，还不如退回不带版本的默认名。
	if !alnum {
		return ""
	}
	return b.String()
}
