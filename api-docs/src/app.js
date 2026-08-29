// 纯静态站，没有框架。四件事：主题、复制、导航高亮、过滤。
(function () {
  'use strict';
  var root = document.documentElement;
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');

  /* ── 亮暗主题 ── */
  function label() {
    var dark = root.classList.contains('dark');
    document.querySelectorAll('.tlabel').forEach(function (n) { n.textContent = dark ? '亮色' : '暗色'; });
    document.querySelectorAll('.theme').forEach(function (b) {
      b.setAttribute('title', dark ? '切换到亮色' : '切换到暗色');
      b.setAttribute('aria-pressed', String(dark));
    });
  }
  function toggle() {
    root.classList.toggle('dark');
    try { localStorage.setItem('ah-theme', root.classList.contains('dark') ? 'dark' : 'light'); } catch (e) {}
    label();
  }
  document.querySelectorAll('.theme').forEach(function (b) { b.addEventListener('click', toggle); });
  label();

  /* ── 一键复制 ── */
  document.querySelectorAll('.copy').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var src = document.getElementById(btn.dataset.copy);
      if (!src) return;
      var text = src.innerText;
      var done = function (ok) {
        var t = btn.querySelector('.copy-txt') || btn;
        t.textContent = ok ? '已复制' : '复制失败';
        btn.classList.toggle('done', ok);
        setTimeout(function () { t.textContent = '复制'; btn.classList.remove('done'); }, 1600);
      };
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(function () { done(true); }, function () { done(fallback(text)); });
      } else {
        done(fallback(text));
      }
    });
  });
  // file:// 与 http 明文下 clipboard API 不可用，退回 execCommand
  function fallback(text) {
    try {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.cssText = 'position:fixed;top:-1000px;opacity:0';
      document.body.appendChild(ta);
      ta.select();
      var ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch (e) { return false; }
  }

  /* ── 导航高亮 ── */
  var links = Array.prototype.slice.call(document.querySelectorAll('.navlink'));
  var byHash = {};
  // 共用端点在多个分组里各有一条链接，所以一个 href 可能对应多个 a
  links.forEach(function (a) {
    var h = a.getAttribute('href');
    (byHash[h] = byHash[h] || []).push(a);
  });
  var targets = Object.keys(byHash)
    .map(function (h) { return document.getElementById(h.slice(1)); })
    .filter(Boolean);

  var current = null;
  var nav = document.querySelector('.nav');
  function activate(el) {
    var group = byHash['#' + el.id];
    if (!group || group[0] === current) return;
    links.forEach(function (l) { l.classList.remove('on'); });
    group.forEach(function (a) {
      a.classList.add('on');
      var g = a.closest('.navgroup');
      if (g) { var head = g.querySelector('.lvl1'); if (head && head !== a) head.classList.add('on'); }
    });
    current = group[0];
    var a = group[0];
    if (nav && (a.offsetTop < nav.scrollTop || a.offsetTop > nav.scrollTop + nav.clientHeight - 40)) {
      nav.scrollTop = a.offsetTop - nav.clientHeight / 2;
    }
  }
  var stream = document.querySelector('.stream');
  if ('IntersectionObserver' in window && targets.length) {
    var visible = new Set();
    var io = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (e) {
          if (e.isIntersecting) visible.add(e.target); else visible.delete(e.target);
        });
        var list = Array.prototype.slice.call(visible).sort(function (a, b) {
          return a.getBoundingClientRect().top - b.getBoundingClientRect().top;
        });
        if (list.length) activate(list[0]);
      },
      { root: stream, rootMargin: '-8% 0px -72% 0px', threshold: 0 }
    );
    targets.forEach(function (t) { io.observe(t); });
  }

  /* ── 过滤 ── */
  var filter = document.getElementById('filter');
  if (filter) {
    filter.addEventListener('input', function () {
      var q = filter.value.trim().toLowerCase();
      document.querySelectorAll('.navlink.lvl2').forEach(function (a) {
        a.hidden = q ? (a.dataset.search || a.textContent).toLowerCase().indexOf(q) === -1 : false;
      });
      document.querySelectorAll('.navgroup').forEach(function (g) {
        var any = g.querySelector('.navlink.lvl2:not([hidden])');
        var head = g.querySelector('.lvl1');
        if (head) head.hidden = q ? !any : false;
      });
      document.querySelectorAll('.nav > .navlink.lvl1').forEach(function (a) { a.hidden = !!q; });
    });
  }

  /* ── 移动端抽屉 ── */
  var rail = document.getElementById('rail');
  var menu = document.getElementById('menu');
  var scrim = document.getElementById('scrim');
  function setOpen(open) {
    if (!rail) return;
    rail.classList.toggle('open', open);
    if (scrim) scrim.hidden = !open;
    if (menu) menu.setAttribute('aria-expanded', String(open));
  }
  if (menu) menu.addEventListener('click', function () { setOpen(!rail.classList.contains('open')); });
  if (scrim) scrim.addEventListener('click', function () { setOpen(false); });
  if (rail) rail.addEventListener('click', function (e) { if (e.target.closest('.navlink')) setOpen(false); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') setOpen(false); });

  // 正文在 .stream 里滚，浏览器默认的 hash 跳转对滚动容器也生效，
  // 但载入时 hash 已经在 URL 上的情况要自己补一次
  function jump(hash, smooth) {
    var el = hash && document.getElementById(hash.slice(1));
    if (!el || !stream) return false;
    el.scrollIntoView({ behavior: smooth && !reduce.matches ? 'smooth' : 'auto', block: 'start' });
    return true;
  }
  if (location.hash) setTimeout(function () { jump(location.hash, false); }, 60);
  document.addEventListener('click', function (e) {
    var a = e.target.closest('a[href^="#"]');
    if (!a) return;
    var h = a.getAttribute('href');
    if (h.length > 1 && jump(h, true)) {
      e.preventDefault();
      history.replaceState(null, '', h);
    }
  });

  // reduce 下平滑滚动也关掉（CSS 已关，这里管 JS 触发的那部分）
  if (reduce.matches && stream) stream.style.scrollBehavior = 'auto';
})();
