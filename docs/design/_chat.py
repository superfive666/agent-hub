# -*- coding: utf-8 -*-
import os
OUT = os.path.dirname(os.path.abspath(__file__))

HEAD = '''<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <script src="./support.js"></script>
</head>
<body>
<x-dc>
<helmet>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap">
  <style>
  /* ── 亮色（默认） ───────────────────────────────── */
  :root{
    --bg:#f6f3ee; --panel:#efeae2; --surface:#ffffff; --raised:#faf8f4;
    --text:#292521; --text2:#6e675e; --text3:#a49c90;
    --line:#e7e0d5; --line2:#f0ebe2;
    --agent:#12897a; --agent-ink:#0d6c60; --agent-soft:#dff0ec; --agent-ring:#a8d8cf;
    --human:#c4642c; --human-ink:#9d4d1d; --human-soft:#fce8d7; --human-ring:#eab98d;
    --alert:#b23c2c; --alert-soft:#fbe3de;
    --warn:#8f6f18;  --warn-soft:#f9eed2;
    --shadow:0 1px 2px rgba(41,37,33,.05), 0 8px 24px -12px rgba(41,37,33,.14);
    --shadow-sm:0 1px 2px rgba(41,37,33,.06);
  }
  /* ── 暗色 ──────────────────────────────────────── */
  .dark{
    --bg:#151719; --panel:#1b1e21; --surface:#212528; --raised:#272b2f;
    --text:#ece7e0; --text2:#a49d94; --text3:#6d675f;
    --line:#31363a; --line2:#282c30;
    --agent:#4ec8b2; --agent-ink:#7fdbc9; --agent-soft:#16302d; --agent-ring:#2d6b60;
    --human:#f0a069; --human-ink:#f5bb90; --human-soft:#35251a; --human-ring:#8a5a33;
    --alert:#f07f6b; --alert-soft:#3a201c;
    --warn:#e0b757;  --warn-soft:#332a13;
    --shadow:0 1px 2px rgba(0,0,0,.3), 0 10px 28px -14px rgba(0,0,0,.55);
    --shadow-sm:0 1px 2px rgba(0,0,0,.28);
  }
  :root, .dark{
    --r-bub:20px; --r-card:16px; --r-ctl:11px; --r-pill:999px;
    --ui:'Manrope',-apple-system,'PingFang SC','Noto Sans SC','Microsoft YaHei',sans-serif;
    --mono:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;
  }
  *{box-sizing:border-box}
  body{margin:0;font-family:var(--ui);-webkit-font-smoothing:antialiased;text-wrap:pretty}
  a{color:var(--human)} a:hover{color:var(--human-ink)}
  .app{display:flex;background:var(--bg);color:var(--text)}

  /* ── 左侧会话栏 ── */
  .rail{width:264px;flex-shrink:0;background:var(--panel);display:flex;flex-direction:column;
        border-right:1px solid var(--line)}
  .brand{padding:20px 18px 14px;display:flex;align-items:center;gap:10px}
  .brand .mk{width:30px;height:30px;border-radius:10px;background:var(--agent);color:#fff;
             display:flex;align-items:center;justify-content:center;font:800 13px/1 var(--ui)}
  .brand b{font:800 15px/1 var(--ui);letter-spacing:-.02em}
  .seg{display:flex;gap:3px;margin:0 14px 12px;background:var(--bg);padding:3px;border-radius:var(--r-ctl)}
  .seg span{flex:1;text-align:center;padding:7px 0;border-radius:8px;font:600 12px/1 var(--ui);color:var(--text2)}
  .seg span.on{background:var(--surface);color:var(--text);box-shadow:var(--shadow-sm)}
  .convo{display:flex;gap:11px;padding:11px 13px;margin:0 8px;border-radius:var(--r-ctl);align-items:flex-start}
  .convo.on{background:var(--surface);box-shadow:var(--shadow-sm)}
  .convo .tt{font:600 12.5px/1.35 var(--ui);display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .convo .pv{font:400 11px/1.4 var(--ui);color:var(--text3);display:block;overflow:hidden;
             text-overflow:ellipsis;white-space:nowrap;margin-top:3px}
  .railfoot{margin-top:auto;padding:12px 14px;display:flex;flex-direction:column;gap:9px}

  /* ── 主区 ── */
  .main{flex-grow:1;display:flex;flex-direction:column;min-width:0}
  .head{padding:16px 24px;border-bottom:1px solid var(--line);display:flex;align-items:center;gap:14px;
        background:color-mix(in oklab, var(--bg) 82%, transparent)}
  .head h1{margin:0;font:700 18px/1.25 var(--ui);letter-spacing:-.02em}
  .head .sub{font:400 11.5px/1.4 var(--ui);color:var(--text3);margin-top:4px}
  .head .sp{margin-left:auto;display:flex;gap:8px;align-items:center}
  .body{flex-grow:1;display:flex;min-width:0}
  .stream{flex-grow:1;padding:22px 24px;display:flex;flex-direction:column;gap:16px;min-width:0}
  .aside{width:290px;flex-shrink:0;border-left:1px solid var(--line);padding:20px;
         display:flex;flex-direction:column;gap:14px;background:var(--panel)}

  /* ── 原子 ── */
  .av{border-radius:var(--r-pill);display:flex;align-items:center;justify-content:center;flex-shrink:0;
      font:700 12px/1 var(--ui);width:36px;height:36px;position:relative}
  .av.a{background:var(--agent-soft);color:var(--agent-ink)}
  .av.p{background:var(--agent);color:#fff;box-shadow:0 0 0 3px var(--agent-ring)}
  .av.h{background:var(--human);color:#fff}
  .av.sm{width:28px;height:28px;font-size:10.5px}
  .av.xs{width:22px;height:22px;font-size:9px}
  .st{position:absolute;right:-1px;bottom:-1px;width:11px;height:11px;border-radius:var(--r-pill);
      border:2.5px solid var(--bg)}
  .st.on{background:#2fbf8f} .st.off{background:var(--text3)}
  .chip{display:inline-flex;align-items:center;gap:5px;padding:4px 10px;border-radius:var(--r-pill);
        font:600 11px/1 var(--ui);background:var(--raised);color:var(--text2);white-space:nowrap}
  .chip.a{background:var(--agent-soft);color:var(--agent-ink)}
  .chip.h{background:var(--human-soft);color:var(--human-ink)}
  .chip.w{background:var(--warn-soft);color:var(--warn)}
  .chip.al{background:var(--alert-soft);color:var(--alert)}
  .chip.solid{background:var(--text);color:var(--bg)}
  .btn{display:inline-flex;align-items:center;gap:7px;padding:9px 16px;border-radius:var(--r-ctl);
       font:600 12.5px/1 var(--ui);background:var(--surface);color:var(--text);
       box-shadow:var(--shadow-sm);border:1px solid var(--line)}
  .btn.pri{background:var(--agent);color:#fff;border-color:transparent}
  .btn.gh{background:transparent;box-shadow:none;border-color:transparent;color:var(--text2)}
  .card{background:var(--surface);border-radius:var(--r-card);box-shadow:var(--shadow);
        border:1px solid var(--line2)}
  .card .hd{padding:13px 16px 0;font:700 11px/1 var(--ui);letter-spacing:.07em;
            color:var(--text3);text-transform:uppercase}
  .card .bd{padding:14px 16px;display:flex;flex-direction:column;gap:11px}
  .kv{display:flex;align-items:center;gap:8px;font:500 12px/1.5 var(--ui);color:var(--text2)}
  .kv b{margin-left:auto;color:var(--text);font-weight:600}
  .mono{font-family:var(--mono)}
  .sep{height:1px;background:var(--line2)}
  .lbl{font:700 10.5px/1 var(--ui);letter-spacing:.07em;color:var(--text3);text-transform:uppercase}
  .in{background:var(--surface);border:1px solid var(--line);border-radius:var(--r-ctl);
      padding:11px 14px;font:500 13px/1.5 var(--ui);color:var(--text)}

  /* ── 气泡 ── */
  .msg{display:flex;gap:11px;align-items:flex-end;max-width:74%}
  .msg.me{margin-left:auto;flex-direction:row-reverse}
  .bub{padding:12px 16px;border-radius:var(--r-bub);font:400 13.5px/1.65 var(--ui);
       background:var(--surface);color:var(--text);box-shadow:var(--shadow-sm);
       border-bottom-left-radius:6px}
  .bub.me{background:var(--human);color:#fff;border-bottom-left-radius:var(--r-bub);
          border-bottom-right-radius:6px}
  .bub.pri{border-left:2.5px solid var(--agent)}
  .bub.watch{background:transparent;border:1px dashed var(--line);box-shadow:none;color:var(--text2)}
  .who{display:flex;align-items:center;gap:7px;margin-bottom:6px;font:600 11.5px/1 var(--ui)}
  .who .t{margin-left:6px;font:500 10.5px/1 var(--ui);color:var(--text3)}
  .msg.me .who{justify-content:flex-end}
  .sys{align-self:center;padding:6px 14px;border-radius:var(--r-pill);background:var(--raised);
       font:600 11px/1 var(--ui);color:var(--text3)}
  .at{color:var(--agent-ink);font-weight:700}
  .bub.me .at{color:#ffe6d2}
  </style>
</helmet>
'''
FOOT = '</x-dc>\n</body>\n</html>\n'

def ic(p,w=16,sw=1.8):
    return ('<svg width="%d" height="%d" viewBox="0 0 24 24" fill="none" stroke="currentColor" '
            'stroke-width="%s" stroke-linecap="round" stroke-linejoin="round">%s</svg>')%(w,w,sw,p)
I={
 'chat': ic('<path d="M20.5 11.4a7.9 7.9 0 0 1-8.5 7.9 8.9 8.9 0 0 1-3.6-.8L3.5 20l1.3-4.4a7.7 7.7 0 0 1-1.3-4.2A7.9 7.9 0 0 1 12 3.5a7.9 7.9 0 0 1 8.5 7.9z"/>'),
 'cal':  ic('<rect x="3.5" y="5" width="17" height="15.5" rx="4"/><path d="M3.5 10h17M8.5 3v4M15.5 3v4"/>'),
 'bot':  ic('<rect x="3.5" y="7.5" width="17" height="13" rx="5"/><path d="M12 7.5V4M9 13.5h.01M15 13.5h.01M9.8 17.2c1.4.9 3 .9 4.4 0"/>'),
 'cog':  ic('<circle cx="12" cy="12" r="3.4"/><path d="M12 2.6v3M12 18.4v3M21.4 12h-3M5.6 12h-3M18.6 5.4l-2 2M7.4 16.6l-2 2M18.6 18.6l-2-2M7.4 7.4l-2-2"/>'),
 'send': ic('<path d="M4.5 12 20 5l-4 7 4 7z"/>'),
 'plus': ic('<path d="M12 5.5v13M5.5 12h13"/>',15),
 'chk':  ic('<path d="M4.5 12.5 9.5 17.5 20 6.5"/>',14),
 'alert':ic('<path d="M12 8.5v5M12 17h.01"/><rect x="2.6" y="2.6" width="18.8" height="18.8" rx="6"/>',14),
 'left': ic('<path d="M14.5 5.5 8 12l6.5 6.5"/>',17),
 'right':ic('<path d="M9.5 5.5 16 12l-6.5 6.5"/>',17),
 'sun':  ic('<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M21.5 12h-2M4.5 12h-2M18.4 5.6l-1.4 1.4M7 17l-1.4 1.4M18.4 18.4 17 17M7 7 5.6 5.6"/>',14),
 'moon': ic('<path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"/>',14),
 'dot3': ic('<circle cx="5" cy="12" r="1.4" fill="currentColor"/><circle cx="12" cy="12" r="1.4" fill="currentColor"/><circle cx="19" cy="12" r="1.4" fill="currentColor"/>',16),
 'link': ic('<path d="M10.5 13.5a4.6 4.6 0 0 0 6.9.5l2.8-2.8a4.6 4.6 0 0 0-6.5-6.5L12.1 6.3"/><path d="M13.5 10.5a4.6 4.6 0 0 0-6.9-.5l-2.8 2.8a4.6 4.6 0 0 0 6.5 6.5l1.6-1.6"/>',14),
 'bell': ic('<path d="M18 8.6a6 6 0 1 0-12 0c0 6-2.5 7.4-2.5 7.4h17S18 14.6 18 8.6z"/><path d="M13.7 19.8a2 2 0 0 1-3.4 0"/>',14),
}

def av(kind, txt, size='', status=None):
    s=('<span class="st %s"></span>'%status) if status else ''
    return '<span class="av %s %s">%s%s</span>'%(kind,size,txt,s)

def write(name, root_style, inner, dark=False):
    cls = 'dark' if dark else ''
    open(os.path.join(OUT, name+'.dc.html'),'w',encoding='utf-8').write(
        HEAD + '<div class="%s" style="%s">%s</div>\n'%(cls, root_style, inner) + FOOT)
