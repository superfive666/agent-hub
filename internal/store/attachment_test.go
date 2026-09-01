package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// 用例按需求写：
//   1. 附件跟着帖子走 —— 帖子发出去了，附件就在上面；帖子没发成，附件不该被认领
//   2. 一个附件只属于一条帖子，而且只有传它的那个人能挂它
//   3. GC 只清「超过 TTL 的孤儿」，且删行 ≠ 删文件（去重）

func mkUpload(t *testing.T, s *store.Store, sha string, who domain.AgentID) store.Attachment {
	t.Helper()
	kind := "agent"
	if who == "" {
		kind = "admin"
	}
	a, err := s.CreateAttachment(context.Background(), store.NewAttachment{
		SHA256: sha, SizeBytes: 12, ContentType: "text/plain; charset=utf-8",
		Filename: "报告.txt", UploaderKind: kind, UploaderID: who,
	})
	if err != nil {
		t.Fatalf("建附件: %v", err)
	}
	return a
}

// 64 个十六进制字符。库上有 CHECK，随便给个字符串是插不进去的。
func sha(seed byte) string {
	out := make([]byte, 64)
	const hex = "0123456789abcdef"
	for i := range out {
		out[i] = hex[(int(seed)+i)%16]
	}
	return string(out)
}

func TestAttachmentsRideAlongWithThePost(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")

	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: agent, Body: "带图的广播"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	up := mkUpload(t, s, sha(1), agent)

	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: "产物在附件里", AttachmentIDs: []string{up.ID},
	}); err != nil {
		t.Fatalf("回帖带附件: %v", err)
	}

	detail, err := s.ThreadDetail(ctx, threadID)
	if err != nil {
		t.Fatalf("读 thread: %v", err)
	}
	last := detail.Posts[len(detail.Posts)-1]
	if len(last.Attachments) != 1 {
		t.Fatalf("最后一条帖子应该带 1 个附件，得到 %d", len(last.Attachments))
	}
	got := last.Attachments[0]
	if got.ID != up.ID || got.Filename != "报告.txt" || got.SizeBytes != 12 || got.SHA256 != sha(1) {
		t.Errorf("附件元数据对不上: %+v", got)
	}

	// 没有附件的帖子必须是空数组而不是 null —— 契约里写死了它一定在，
	// 前端才不用在每个渲染点写一次 `?? []`。
	first := detail.Posts[0]
	if first.Attachments == nil {
		t.Error("没有附件的帖子，attachments 应该是空数组而不是 nil")
	}
	if len(first.Attachments) != 0 {
		t.Errorf("首帖不该有附件，得到 %d 个", len(first.Attachments))
	}
}

// 一个附件只能挂到一条帖子上。第二次挂应该整笔失败 ——
// 不是「悄悄不挂」，也不是把它从第一条帖子上搬走。
func TestAttachmentCannotBeClaimedTwice(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")
	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: agent, Body: "开个头"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	up := mkUpload(t, s, sha(2), agent)

	firstPost, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: "第一次", AttachmentIDs: []string{up.ID},
	})
	if err != nil {
		t.Fatalf("第一次挂: %v", err)
	}

	_, err = s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: "第二次", AttachmentIDs: []string{up.ID},
	})
	if !errors.Is(err, store.ErrAttachmentNotClaimable) {
		t.Fatalf("重复挂应该报 ErrAttachmentNotClaimable，得到 %v", err)
	}

	// 整笔回滚：那条「第二次」的帖子不该留下来
	if n := countRows(t, s, `SELECT count(*) FROM post WHERE body = '第二次'`); n != 0 {
		t.Errorf("挂附件失败时整条帖子都该回滚，却留下了 %d 条", n)
	}
	// 也不该把附件从第一条帖子上搬走
	if n := countRows(t, s,
		`SELECT count(*) FROM attachment WHERE id = $1 AND post_id = $2`, up.ID, firstPost); n != 1 {
		t.Error("附件应该还挂在第一条帖子上")
	}
}

// agent A 不能把 agent B 传了一半的东西挂到自己的帖子上。
func TestAgentCannotClaimSomeoneElsesUpload(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	alice := mkAgent(t, s, "alice")
	bob := mkAgent(t, s, "bob")
	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: alice, Body: "开个头"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	upByAlice := mkUpload(t, s, sha(3), alice)

	_, err = s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: bob,
		Body: "这不是我传的", AttachmentIDs: []string{upByAlice.ID},
	})
	if !errors.Is(err, store.ErrAttachmentNotClaimable) {
		t.Fatalf("挂别人的上传应该被拒，得到 %v", err)
	}
	if n := countRows(t, s, `SELECT count(*) FROM post WHERE body = '这不是我传的'`); n != 0 {
		t.Errorf("被拒时帖子该整条回滚，却留下了 %d 条", n)
	}
}

// 管理员传的附件 agent 挂不上，反过来也一样 —— uploader_kind 也是判据之一。
func TestAdminUploadIsNotClaimableByAgent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "worker-a")
	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: agent, Body: "开个头"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	byAdmin := mkUpload(t, s, sha(4), "")

	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: "偷用管理员的", AttachmentIDs: []string{byAdmin.ID},
	}); !errors.Is(err, store.ErrAttachmentNotClaimable) {
		t.Fatalf("agent 挂管理员的上传应该被拒，得到 %v", err)
	}

	// 管理员自己挂得上
	if _, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "admin",
		Body: "我发的文件", AttachmentIDs: []string{byAdmin.ID},
	}); err != nil {
		t.Fatalf("管理员挂自己的上传应该成功: %v", err)
	}
}

// 一串 id 里只要有一个挂不上，整笔就该失败。
// 「发出去的帖子少了两个附件、但没报错」正是那种谁都不会发现的失败。
func TestPartialClaimFailsWholePost(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")
	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: agent, Body: "开个头"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	good := mkUpload(t, s, sha(5), agent)

	_, err = s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: "两个附件一个是假的",
		// 第二个是个根本不是 uuid 的字符串 —— 它来自 HTTP 请求体，
		// 必须归到「挂不上」，而不是让 ::uuid 转换报 22P02 变成 500。
		AttachmentIDs: []string{good.ID, "不是-uuid"},
	})
	if !errors.Is(err, store.ErrAttachmentNotClaimable) {
		t.Fatalf("有一个挂不上就该整笔失败，得到 %v", err)
	}
	if n := countRows(t, s,
		`SELECT count(*) FROM attachment WHERE id = $1 AND post_id IS NULL`, good.ID); n != 1 {
		t.Error("整笔回滚之后，那个合法的附件应该还是未认领状态")
	}
}

// 同一个 id 在一次请求里写了两遍，是一次合法的重复，不该被判成失败。
func TestDuplicateIDsInOneRequestAreFine(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")
	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{Author: agent, Body: "开个头"})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	up := mkUpload(t, s, sha(6), agent)

	postID, err := s.AppendPost(ctx, store.AppendPostParams{
		ThreadID: threadID, AuthorKind: "agent", AuthorID: agent,
		Body: "写了两遍", AttachmentIDs: []string{up.ID, up.ID},
	})
	if err != nil {
		t.Fatalf("同一个 id 传两遍应该成功: %v", err)
	}
	if n := countRows(t, s,
		`SELECT count(*) FROM attachment WHERE post_id = $1`, postID); n != 1 {
		t.Errorf("应该只挂上 1 个附件，得到 %d", n)
	}
}

// 广播的首帖也能带附件。
func TestTweetOpeningPostCarriesAttachments(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")
	up := mkUpload(t, s, sha(7), agent)

	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{
		Author: agent, Body: "看这张图", AttachmentIDs: []string{up.ID},
	})
	if err != nil {
		t.Fatalf("发带附件的广播: %v", err)
	}
	detail, err := s.ThreadDetail(ctx, threadID)
	if err != nil {
		t.Fatalf("读 thread: %v", err)
	}
	if len(detail.Posts) != 1 || len(detail.Posts[0].Attachments) != 1 {
		t.Fatalf("首帖应该带 1 个附件，得到 %+v", detail.Posts)
	}
}

// 帖子被删（thread 删掉时级联），附件行要跟着走 —— 否则留下一堆
// 指向不存在帖子的行，GC 又不会碰它们（post_id 不为空），永远清不掉。
func TestDeletingThreadCascadesToAttachments(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")
	up := mkUpload(t, s, sha(8), agent)
	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{
		Author: agent, Body: "待会儿删掉", AttachmentIDs: []string{up.ID},
	})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx, `DELETE FROM thread WHERE id = $1`, threadID); err != nil {
		t.Fatalf("删 thread: %v", err)
	}
	if n := countRows(t, s, `SELECT count(*) FROM attachment WHERE id = $1`, up.ID); n != 0 {
		t.Error("thread 删掉之后附件行应该跟着级联删掉")
	}
}

// GC 只清超过 TTL 的孤儿。两步上传中间隔着一段时间 —— agent 先传三个文件再发帖，
// 不看时间的清理会在它发帖之前把前两个删掉。
func TestPurgeOrphansOnlyTouchesOldUnclaimedOnes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")

	fresh := mkUpload(t, s, sha(9), agent)  // 刚传上来，还没发帖
	stale := mkUpload(t, s, sha(10), agent) // 传上来之后那条帖子没发成
	claimed := mkUpload(t, s, sha(11), agent)

	threadID, err := s.CreateTweet(ctx, store.CreateTweetParams{
		Author: agent, Body: "已经挂上了", AttachmentIDs: []string{claimed.ID},
	})
	if err != nil {
		t.Fatalf("发广播: %v", err)
	}
	_ = threadID

	// 把 stale 拨老到 TTL 之外
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE attachment SET created_at = now() - interval '48 hours' WHERE id = $1`,
		stale.ID); err != nil {
		t.Fatalf("改时间: %v", err)
	}
	// 已认领的那个也拨老 —— 它不该因为「老」被清掉，判据是 post_id 为空
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE attachment SET created_at = now() - interval '48 hours' WHERE id = $1`,
		claimed.ID); err != nil {
		t.Fatalf("改时间: %v", err)
	}

	n, err := s.PurgeOrphanAttachments(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("清孤儿: %v", err)
	}
	if n != 1 {
		t.Errorf("应该只清掉 1 条，实际 %d", n)
	}
	for _, c := range []struct {
		name string
		id   string
		want int
	}{
		{"刚传上来的", fresh.ID, 1},
		{"超时未认领的", stale.ID, 0},
		{"已经挂在帖子上的", claimed.ID, 1},
	} {
		if got := countRows(t, s, `SELECT count(*) FROM attachment WHERE id = $1`, c.id); got != c.want {
			t.Errorf("%s：应该剩 %d 行，实际 %d", c.name, c.want, got)
		}
	}
}

// 去重让「删行」和「删文件」分开：同一份内容被两条行引用时，
// 删掉其中一条不能删磁盘上那份 —— 否则另一条立刻变成点下去 404 的附件。
func TestUnreferencedSHAsKeepsBlobsThatOthersStillUse(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")

	shared := sha(12)
	lonely := sha(13)
	gone := sha(14) // 磁盘上有，库里从来没有过

	a1 := mkUpload(t, s, shared, agent)
	_ = mkUpload(t, s, shared, agent) // 同一份内容的第二条行
	a3 := mkUpload(t, s, lonely, agent)

	free, err := s.UnreferencedSHAs(ctx, []string{shared, lonely, gone})
	if err != nil {
		t.Fatalf("查失联 blob: %v", err)
	}
	if len(free) != 1 || free[0] != gone {
		t.Fatalf("此刻只有 %s 该被判为可删，得到 %v", gone[:8], free)
	}

	// 删掉共享内容的一条行 —— 另一条还在，磁盘上那份仍然不能删
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM attachment WHERE id = $1`, a1.ID); err != nil {
		t.Fatalf("删行: %v", err)
	}
	free, err = s.UnreferencedSHAs(ctx, []string{shared, lonely})
	if err != nil {
		t.Fatalf("查失联 blob: %v", err)
	}
	if len(free) != 0 {
		t.Errorf("还有别的行引用着，不该判为可删：%v", free)
	}

	// 两条行都没了，才轮到磁盘
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM attachment WHERE sha256 = $1`, shared); err != nil {
		t.Fatalf("删行: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM attachment WHERE id = $1`, a3.ID); err != nil {
		t.Fatalf("删行: %v", err)
	}
	free, err = s.UnreferencedSHAs(ctx, []string{shared, lonely})
	if err != nil {
		t.Fatalf("查失联 blob: %v", err)
	}
	if len(free) != 2 {
		t.Errorf("两份内容都没人引用了，应该都可删，得到 %v", free)
	}
}

func TestCountAttachmentsDeduplicatesBySHA(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	agent := mkAgent(t, s, "uploader")

	same := sha(15)
	mkUpload(t, s, same, agent)
	mkUpload(t, s, same, agent) // 同一份内容，第二条元数据行
	mkUpload(t, s, sha(16), agent)

	// 磁盘上是两份内容、24 字节。报三份会让运维拿着一个和 du 对不上的数字去查盘。
	count, bytes, err := s.CountAttachments(ctx)
	if err != nil {
		t.Fatalf("统计: %v", err)
	}
	if count != 2 {
		t.Errorf("按内容去重后应该是 2 份，得到 %d", count)
	}
	if bytes != 24 {
		t.Errorf("应该是 24 字节，得到 %d", bytes)
	}
}
