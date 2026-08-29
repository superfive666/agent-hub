/* agent-hub developer-docs —— 零依赖交互：主题切换、代码复制、当前页高亮。
   主题的首帧由每个页面 <head> 里的内联脚本决定（避免闪白），这里只负责切换与持久化。 */
(function () {
  'use strict';

  var root = document.documentElement;

  function stored() {
    try { return localStorage.getItem('agent-hub-theme'); } catch (e) { return null; }
  }
  function persist(v) {
    try { localStorage.setItem('agent-hub-theme', v); } catch (e) { /* 隐私模式下静默失败 */ }
  }
  function isDark() { return root.classList.contains('dark'); }

  function paintToggle(btn) {
    var dark = isDark();
    btn.textContent = dark ? '☀' : '☾';
    btn.setAttribute('aria-label', dark ? '切换到亮色主题' : '切换到暗色主题');
    btn.setAttribute('title', dark ? '亮色' : '暗色');
  }

  document.querySelectorAll('[data-theme-toggle]').forEach(function (btn) {
    paintToggle(btn);
    btn.addEventListener('click', function () {
      root.classList.toggle('dark');
      persist(isDark() ? 'dark' : 'light');
      document.querySelectorAll('[data-theme-toggle]').forEach(paintToggle);
    });
  });

  // 没有显式选择过的话，跟随系统
  if (window.matchMedia) {
    var mq = window.matchMedia('(prefers-color-scheme: dark)');
    var onChange = function (e) {
      if (stored()) return;
      root.classList.toggle('dark', e.matches);
      document.querySelectorAll('[data-theme-toggle]').forEach(paintToggle);
    };
    if (mq.addEventListener) mq.addEventListener('change', onChange);
    else if (mq.addListener) mq.addListener(onChange);
  }

  // 代码块复制
  document.querySelectorAll('.code').forEach(function (block) {
    var pre = block.querySelector('pre');
    var btn = block.querySelector('.copy');
    if (!pre || !btn) return;
    btn.addEventListener('click', function () {
      var text = pre.innerText;
      var done = function () {
        var old = btn.textContent;
        btn.textContent = '已复制';
        setTimeout(function () { btn.textContent = old; }, 1400);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, fallback);
      } else { fallback(); }
      function fallback() {
        var ta = document.createElement('textarea');
        ta.value = text; ta.setAttribute('readonly', '');
        ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta); ta.select();
        try { document.execCommand('copy'); done(); } catch (e) { btn.textContent = '复制失败'; }
        document.body.removeChild(ta);
      }
    });
  });

  // 当前页高亮（不依赖服务端渲染）
  var here = location.pathname.split('/').pop() || 'index.html';
  document.querySelectorAll('.nav a').forEach(function (a) {
    var href = a.getAttribute('href');
    if (href === here || (here === 'index.html' && href === './')) {
      a.setAttribute('aria-current', 'page');
    }
  });
})();
