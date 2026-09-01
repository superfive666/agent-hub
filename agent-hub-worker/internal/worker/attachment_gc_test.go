package worker_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/agent-hub-worker/internal/worker"
	"github.com/superfive666/agent-hub/internal/blobstore"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// 附件 GC 的需求就三条（ADR-0011 第五条）：
//   1. 传上来但一直没发帖的，超过 TTL 才清 —— 早一秒都会把正在两步上传中间
//      的那个人的文件删掉
//   2. thread 删了，磁盘上那份也要跟着走，不然磁盘只涨不降
//   3. **删行不等于删文件**：去重让一份内容可能被多条行引用

// gcSetup 造一个开了附件的 worker 环境。
func gcSetup(t *testing.T, ttl time.Duration) (*store.Store, blobstore.Store, *worker.Worker) {
	t.Helper()
	st := newStore(t)
	bs := blobstore.Store{Dir: t.TempDir(), MaxBytes: 1 << 20}
	w := worker.New(st, &spy{}, worker.Config{
		IdleInterval: 10 * time.Millisecond,
		// 只想跑附件 GC，别让补投和 inbox 清理跟着响
		RenotifyEvery: -1,
		PurgeEvery:    15 * time.Millisecond,
		Blobs:         bs,
		AttachmentTTL: ttl,
	}, nil)
	return st, bs, w
}

// runGC 把 worker 跑够几个 purge 周期再停。
func runGC(t *testing.T, w *worker.Worker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("worker 退出异常: %v", err)
	}
}

// putBlob 落一份内容，并把它的 mtime 拨到 age 之前。
func putBlob(t *testing.T, bs blobstore.Store, content string, age time.Duration) string {
	t.Helper()
	sha, _, err := bs.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("落盘: %v", err)
	}
	if age > 0 {
		old := time.Now().Add(-age)
		p := filepath.Join(bs.Dir, sha[0:2], sha[2:4], sha)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatalf("改 mtime: %v", err)
		}
	}
	return sha
}

func blobExists(t *testing.T, bs blobstore.Store, sha string) bool {
	t.Helper()
	f, _, err := bs.Open(sha)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		t.Fatalf("查 blob: %v", err)
	}
	f.Close()
	return true
}

func mkRow(t *testing.T, st *store.Store, sha string, who domain.AgentID) store.Attachment {
	t.Helper()
	a, err := st.CreateAttachment(context.Background(), store.NewAttachment{
		SHA256: sha, SizeBytes: 8, ContentType: "text/plain; charset=utf-8",
		Filename: "x.txt", UploaderKind: "agent", UploaderID: who,
	})
	if err != nil {
		t.Fatalf("写 attachment 行: %v", err)
	}
	return a
}

func ageRow(t *testing.T, st *store.Store, id string, age time.Duration) {
	t.Helper()
	if _, err := st.DB().ExecContext(context.Background(),
		`UPDATE attachment SET created_at = now() - make_interval(secs => $2) WHERE id = $1`,
		id, age.Seconds()); err != nil {
		t.Fatalf("改 created_at: %v", err)
	}
}

func countRows(t *testing.T, st *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("计数 (%s): %v", q, err)
	}
	return n
}

// 需求 1：两步上传中间那段时间是合法的，GC 不许闯进来。
func TestGCLeavesFreshUnclaimedUploadsAlone(t *testing.T) {
	st, bs, w := gcSetup(t, time.Hour)
	agent, err := st.CreateAgent(context.Background(), "uploader", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}

	// 刚传上来、还没发帖 —— agent 可能正打算再传两个然后一起发
	sha := putBlob(t, bs, "刚传上来的产物", 0)
	row := mkRow(t, st, sha, agent)

	runGC(t, w)

	if countRows(t, st, `SELECT count(*) FROM attachment WHERE id = $1`, row.ID) != 1 {
		t.Error("还在 TTL 内的未认领附件不该被清掉 —— 那会把正在两步上传中间的人坑了")
	}
	if !blobExists(t, bs, sha) {
		t.Error("对应的文件也不该被删")
	}
}

// 需求 1 的另一半：超过 TTL 的孤儿，行和文件都要清干净。
func TestGCRemovesStaleUnclaimedUploads(t *testing.T) {
	st, bs, w := gcSetup(t, time.Hour)
	agent, err := st.CreateAgent(context.Background(), "quitter", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}

	sha := putBlob(t, bs, "那条帖子最终没发出去", 2*time.Hour)
	row := mkRow(t, st, sha, agent)
	ageRow(t, st, row.ID, 2*time.Hour)

	runGC(t, w)

	if countRows(t, st, `SELECT count(*) FROM attachment WHERE id = $1`, row.ID) != 0 {
		t.Error("超过 TTL 的未认领附件行应该被清掉")
	}
	if blobExists(t, bs, sha) {
		t.Error("行清掉之后，没人引用的那份内容也该从磁盘上消失")
	}
}

// 需求 2：thread 删了，磁盘上那份要跟着走。
// 库那边是外键级联，磁盘这边只能靠 GC —— 少了它，磁盘只涨不降。
func TestGCReclaimsDiskAfterThreadDeletion(t *testing.T) {
	st, bs, w := gcSetup(t, time.Hour)
	ctx := context.Background()
	agent, err := st.CreateAgent(ctx, "worker-a", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}

	sha := putBlob(t, bs, "挂在帖子上的产物", 2*time.Hour)
	row := mkRow(t, st, sha, agent)
	threadID, err := st.CreateTweet(ctx, store.CreateTweetParams{
		Author: agent, Body: "带附件的广播", AttachmentIDs: []string{row.ID},
	})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}

	// 挂着的时候谁都不许动它
	runGC(t, w)
	if !blobExists(t, bs, sha) {
		t.Fatal("还挂在帖子上的附件被删了")
	}

	// thread 一删，attachment 行级联消失，磁盘上那份就没人引用了
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM thread WHERE id = $1`, threadID); err != nil {
		t.Fatalf("删 thread: %v", err)
	}
	runGC(t, w)
	if blobExists(t, bs, sha) {
		t.Error("thread 删掉之后，磁盘上那份也该被回收 —— 否则磁盘只涨不降")
	}
}

// 需求 3：去重让一份内容可能被多条行引用。删掉其中一条行就删文件的话，
// 另一条行立刻变成点下去 404 的附件，而且删除本身是成功的，没有任何报错。
func TestGCKeepsBlobsThatAnotherRowStillReferences(t *testing.T) {
	st, bs, w := gcSetup(t, time.Hour)
	ctx := context.Background()
	agent, err := st.CreateAgent(ctx, "duper", "测试", "superfive")
	if err != nil {
		t.Fatal(err)
	}

	// 同一份内容，两条元数据行（同一个产物发到两条 thread）
	sha := putBlob(t, bs, "一模一样的产物", 2*time.Hour)
	stale := mkRow(t, st, sha, agent)  // 这条是孤儿，会被清掉
	keeper := mkRow(t, st, sha, agent) // 这条挂到帖子上，要留着
	ageRow(t, st, stale.ID, 2*time.Hour)

	if _, err := st.CreateTweet(ctx, store.CreateTweetParams{
		Author: agent, Body: "留着这条", AttachmentIDs: []string{keeper.ID},
	}); err != nil {
		t.Fatalf("发广播: %v", err)
	}

	runGC(t, w)

	if countRows(t, st, `SELECT count(*) FROM attachment WHERE id = $1`, stale.ID) != 0 {
		t.Error("孤儿行该被清掉")
	}
	if countRows(t, st, `SELECT count(*) FROM attachment WHERE id = $1`, keeper.ID) != 1 {
		t.Fatal("挂在帖子上的行不该被动")
	}
	if !blobExists(t, bs, sha) {
		t.Error("还有另一条行引用着这份内容，磁盘上那份绝不能删 —— " +
			"删了之后那条帖子的附件点下去就是 404，而且没有任何报错")
	}
}

// 磁盘上有、库里从来没有过的文件（上传写完文件但写库失败），超过 TTL 要收走。
func TestGCRemovesBlobsWithNoRowAtAll(t *testing.T) {
	_, bs, w := gcSetup(t, time.Hour)

	orphanBlob := putBlob(t, bs, "库里从来没记过这份", 2*time.Hour)
	fresh := putBlob(t, bs, "刚落盘、库行还没写完", 0)

	runGC(t, w)

	if blobExists(t, bs, orphanBlob) {
		t.Error("库里没有任何行引用、又过了 TTL 的文件应该被收走")
	}
	// 这一条是那个「先落盘、后写库行」的窗口 —— 删了它就是
	// 上传方拿到 201、下载时 404，而且静默。
	if !blobExists(t, bs, fresh) {
		t.Error("刚落盘的文件绝不能删：上传是先落盘后写库行，" +
			"这一瞬间它本来就没有任何行引用")
	}
}

// 进程在上传中途被杀会留下 .upload-* 临时文件。它们不是合法 sha，
// 扫 blob 那趟看不见它们，得单独收尾，否则只增不减。
func TestGCSweepsAbandonedTempFiles(t *testing.T) {
	_, bs, w := gcSetup(t, time.Hour)

	stale := filepath.Join(bs.Dir, ".upload-killed")
	if err := os.WriteFile(stale, []byte("半个文件"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	inflight := filepath.Join(bs.Dir, ".upload-inflight")
	if err := os.WriteFile(inflight, []byte("正在传"), 0o600); err != nil {
		t.Fatal(err)
	}

	runGC(t, w)

	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Error("上传中断留下的临时文件应该被清掉")
	}
	if _, err := os.Stat(inflight); err != nil {
		t.Errorf("正在传的临时文件被删了: %v", err)
	}
}

// 没配 ATTACHMENT_DIR 的部署，附件 GC 整块跳过，不能因此让 worker 报错或退出。
func TestGCIsANoOpWhenAttachmentsAreOff(t *testing.T) {
	st := newStore(t)
	w := worker.New(st, &spy{}, worker.Config{
		IdleInterval:  10 * time.Millisecond,
		RenotifyEvery: -1,
		PurgeEvery:    15 * time.Millisecond,
		// Blobs 是零值 —— 这台部署没开附件
	}, nil)
	runGC(t, w) // 不 panic、不返回错误就算过
}
