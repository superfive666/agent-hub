# -*- coding: utf-8 -*-
DUST = open('_dust.txt').read()

STYLE = '''  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap">
  <style>
  @property --ang { syntax:'<angle>'; initial-value:0deg; inherits:false; }

  /* ═══ 亮色 · 液态玻璃 ═══ */
  :root{
    --ink:#10222c; --ink2:#4a6472; --ink3:#8aa2ae;
    --hair:rgba(255,255,255,.92); --hair2:rgba(120,160,185,.20);
    --agent:#12a58c; --agent-ink:#07705f; --agent-soft:rgba(18,190,160,.16);
    --human:#e0762a; --human-ink:#a3500f; --human-soft:rgba(255,150,70,.18);
    --alert:#cf4331; --alert-soft:rgba(207,67,49,.14);
    --warn:#a07a10;  --warn-soft:rgba(190,150,30,.16);
    --i1:#5fe3ff; --i2:#9d8cff; --i3:#ff8fd8; --i4:#ffd68a; --i5:#7cf0c8;
    --aura:rgba(90,220,210,.0);
    --pane-bg:linear-gradient(158deg,rgba(255,255,255,.66),rgba(255,255,255,.34) 46%,rgba(255,255,255,.52));
    --pane-bd:rgba(255,255,255,.75);
    --pane-sh:inset 0 1.5px 0 rgba(255,255,255,1),inset 0 -1px 0 rgba(255,255,255,.55),
              inset 0 22px 44px -28px rgba(255,255,255,1),
              inset 0 -30px 52px -34px rgba(60,115,150,.42),
              0 36px 74px -36px rgba(20,55,85,.42),0 2px 10px -5px rgba(20,55,85,.18);
    --inset-bg:rgba(255,255,255,.40); --inset-bd:rgba(255,255,255,.62);
    --inset-sh:inset 0 1px 0 rgba(255,255,255,.95),inset 0 -1px 0 rgba(120,165,195,.22),
               inset 0 12px 26px -20px rgba(255,255,255,.9);
    --chip-bg:rgba(255,255,255,.52);
    --prism-op:.42; --sheen-op:.55; --dust-op:0;
    --pri-grad:linear-gradient(135deg,#a8f0d2,#4fd6b4 52%,#37cbdd);
    --pri-ink:#053a30; --pri-sh:inset 0 1.5px 0 rgba(255,255,255,.9),0 12px 28px -12px rgba(50,200,180,.65);
    --me-grad:linear-gradient(135deg,#ffd9a8,#ff9f5a 55%,#f4813c);
    --me-ink:#4a2004; --me-sh:inset 0 1.5px 0 rgba(255,255,255,.75),0 14px 32px -14px rgba(240,140,70,.6);
  }
  /* ═══ 暗色 · 霓虹光影 ═══ */
  .dark{
    --ink:#eef2fa; --ink2:#94a2b8; --ink3:#5d6a80;
    --hair:rgba(255,255,255,.14); --hair2:rgba(255,255,255,.07);
    --agent:#3ee0c8; --agent-ink:#8af3e2; --agent-soft:rgba(62,224,200,.14);
    --human:#ff9f5a; --human-ink:#ffc08f; --human-soft:rgba(255,159,90,.16);
    --alert:#ff6f5c; --alert-soft:rgba(255,111,92,.14);
    --warn:#f0c25a;  --warn-soft:rgba(240,194,90,.13);
    --i1:#41e0ff; --i2:#a06bff; --i3:#ff5cc8; --i4:#ffb35c; --i5:#3ee0c8;
    --aura:rgba(120,80,255,.42);
    --pane-bg:linear-gradient(158deg,rgba(255,255,255,.085),rgba(255,255,255,.018) 46%,rgba(255,255,255,.055));
    --pane-bd:rgba(255,255,255,.10);
    --pane-sh:inset 0 1px 0 rgba(255,255,255,.16),inset 0 -28px 50px -36px rgba(255,255,255,.14),
              0 44px 96px -44px #000,0 0 74px -24px var(--aura);
    --inset-bg:rgba(255,255,255,.028); --inset-bd:rgba(255,255,255,.06);
    --inset-sh:inset 0 1px 0 rgba(255,255,255,.07);
    --chip-bg:rgba(255,255,255,.06);
    --prism-op:.85; --sheen-op:.16; --dust-op:1;
    --pri-grad:linear-gradient(135deg,#8b5cff,#c94bff 52%,#ff5cc8);
    --pri-ink:#fff; --pri-sh:inset 0 1px 0 rgba(255,255,255,.4),0 14px 34px -12px rgba(170,80,255,.7);
    --me-grad:linear-gradient(135deg,#ffc98a,#ff9f5a 55%,#f07a3c);
    --me-ink:#3a1a04; --me-sh:inset 0 1px 0 rgba(255,255,255,.5),0 16px 38px -14px rgba(255,140,70,.55);
  }
  :root,.dark{
    --r-slab:38px; --r-pane:30px; --r-in:24px; --r-card:20px; --r-bub:24px; --r-pill:999px;
    --ui:'Manrope',-apple-system,'PingFang SC','Noto Sans SC','Microsoft YaHei',sans-serif;
    --mono:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;
    --ease:cubic-bezier(.22,.61,.28,1);
    --blur:30px;
  }
  *{box-sizing:border-box}
  body{margin:0;font-family:var(--ui);-webkit-font-smoothing:antialiased;text-wrap:pretty}
  a{color:var(--human)}

  /* ═══ 舞台：不再是"页面底色"，是一张有天气的背景 ═══ */
  .app{display:flex;gap:16px;padding:22px;color:var(--ink);position:relative;isolation:isolate;
    overflow:hidden;align-items:stretch;
    background:
      radial-gradient(78% 58% at 16% 4%,#c9e9f7 0%,transparent 58%),
      radial-gradient(64% 52% at 86% 10%,#dcdcff 0%,transparent 60%),
      radial-gradient(86% 62% at 62% 104%,#dffaf0 0%,transparent 58%),
      linear-gradient(168deg,#cfe4f0 0%,#e2eff5 42%,#eef8f8 100%)}
  .dark .app,.dark.app{background:
      radial-gradient(58% 48% at 18% 6%,rgba(40,90,255,.20),transparent 60%),
      radial-gradient(52% 44% at 88% 14%,rgba(190,60,255,.18),transparent 62%),
      radial-gradient(74% 56% at 56% 106%,rgba(0,220,190,.13),transparent 58%),
      #05060a}
  /* 焦散光带 + 棱镜色散 */
  .app::before{content:'';position:absolute;inset:-20%;z-index:0;pointer-events:none;opacity:.75;
    background:
      conic-gradient(from 210deg at 30% 20%,transparent 0 18%,rgba(255,255,255,.5) 24%,transparent 30%),
      conic-gradient(from 40deg at 76% 74%,transparent 0 20%,rgba(255,255,255,.38) 26%,transparent 33%),
      linear-gradient(104deg,transparent 40%,rgba(255,120,220,.28) 46%,rgba(120,220,255,.30) 50%,
        rgba(180,255,200,.26) 54%,transparent 60%);
    filter:blur(46px);animation:weather 30s var(--ease) infinite alternate}
  .dark .app::before,.dark.app::before{opacity:.32;filter:blur(70px)}
  /* 尘埃粒子（暗色专用） */
  .app::after{content:'';position:absolute;left:0;top:0;width:2px;height:2px;border-radius:50%;
    z-index:0;pointer-events:none;opacity:var(--dust-op);
    box-shadow:''' + DUST + ''';animation:dust 40s linear infinite}
  .app > *{position:relative;z-index:1}
  @keyframes weather{0%{transform:translate3d(0,0,0) scale(1) rotate(0deg)}
    100%{transform:translate3d(3%,-2.5%,0) scale(1.1) rotate(4deg)}}
  @keyframes dust{from{transform:translate3d(0,0,0)}to{transform:translate3d(-46px,-110px,0)}}

  /* ═══ 厚玻璃板：边缘有厚度，不是一张半透明纸 ═══ */
  .rail,.main{position:relative;border-radius:var(--r-pane);background:var(--pane-bg);
    backdrop-filter:blur(var(--blur)) saturate(190%) brightness(1.04);
    -webkit-backdrop-filter:blur(var(--blur)) saturate(190%) brightness(1.04);
    border:1px solid var(--pane-bd);box-shadow:var(--pane-sh);overflow:hidden}
  /* 棱镜色散边（亮色低调，暗色就是霓虹角度渐变描边） */
  .rail::before,.main::before{content:'';position:absolute;inset:0;border-radius:inherit;padding:1.4px;
    background:conic-gradient(from var(--ang),var(--i1),var(--i2),var(--i3),var(--i4),var(--i5),var(--i1));
    -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
    -webkit-mask-composite:xor;mask-composite:exclude;
    opacity:var(--prism-op);animation:spin 16s linear infinite;pointer-events:none;z-index:2}
  /* 高光扫过：液态感的来源 */
  .rail::after,.main::after{content:'';position:absolute;inset:0;border-radius:inherit;pointer-events:none;
    background:linear-gradient(126deg,transparent 30%,rgba(255,255,255,.85) 46%,transparent 58%);
    opacity:var(--sheen-op);mix-blend-mode:overlay;
    animation:sheen 14s var(--ease) infinite;z-index:2}
  @keyframes spin{to{--ang:360deg}}
  @keyframes sheen{0%,100%{transform:translateX(-28%)}50%{transform:translateX(28%)}}

  .rail{width:278px;flex-shrink:0;display:flex;flex-direction:column;padding:18px 0 14px}
  .main{flex-grow:1;display:flex;flex-direction:column;min-width:0}

  /* ═══ 嵌套内板：板中有板，是这套构图的核心 ═══ */
  .stream,.aside{border-radius:var(--r-in);background:var(--inset-bg);border:1px solid var(--inset-bd);
    box-shadow:var(--inset-sh)}
  .body{flex-grow:1;display:flex;min-width:0;gap:14px;padding:0 14px 14px}
  .stream{flex-grow:1;padding:22px;display:flex;flex-direction:column;gap:15px;min-width:0}
  .aside{width:292px;flex-shrink:0;padding:18px;display:flex;flex-direction:column;gap:13px}
  .head{padding:20px 24px 16px;display:flex;align-items:center;gap:14px;position:relative;z-index:3}
  .head h1{margin:0;font:800 19px/1.25 var(--ui);letter-spacing:-.03em}
  .head .sub{font:500 11.5px/1.45 var(--ui);color:var(--ink3);margin-top:6px}
  .head .sp{margin-left:auto;display:flex;gap:9px;align-items:center}

  /* ═══ 侧栏内容 ═══ */
  .brand{padding:0 20px 16px;display:flex;align-items:center;gap:11px}
  .brand .mk{width:38px;height:38px;border-radius:var(--r-pill);display:flex;align-items:center;
    justify-content:center;font:800 13px/1 var(--ui);color:var(--pri-ink);
    background:var(--pri-grad);box-shadow:var(--pri-sh);animation:breathe 5s var(--ease) infinite}
  .brand b{font:800 15.5px/1 var(--ui);letter-spacing:-.03em}
  .seg{display:flex;gap:3px;margin:0 16px 14px;padding:4px;border-radius:var(--r-pill);
    background:var(--inset-bg);border:1px solid var(--inset-bd);box-shadow:var(--inset-sh)}
  .seg span{flex:1;text-align:center;padding:8px 0;border-radius:var(--r-pill);
    font:700 11.5px/1 var(--ui);color:var(--ink2);transition:.3s var(--ease)}
  .seg span.on{background:var(--pane-bg);color:var(--ink);
    box-shadow:inset 0 1px 0 var(--hair),0 4px 12px -6px rgba(20,55,85,.35);border:1px solid var(--pane-bd)}
  .convo{display:flex;gap:12px;padding:12px 14px;margin:0 10px;border-radius:var(--r-card);
    align-items:flex-start;transition:.32s var(--ease);position:relative}
  .convo:hover{background:var(--chip-bg);transform:translateX(3px)}
  .convo.on{background:var(--inset-bg);border:1px solid var(--inset-bd);box-shadow:var(--inset-sh)}
  .convo .tt{font:700 12.5px/1.35 var(--ui);display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .convo .pv{font:500 11px/1.4 var(--ui);color:var(--ink3);display:block;overflow:hidden;
    text-overflow:ellipsis;white-space:nowrap;margin-top:4px}
  .railfoot{margin-top:auto;padding:12px 16px 0;display:flex;flex-direction:column;gap:10px}

  /* ═══ 原子：一切都是胶囊 ═══ */
  .av{border-radius:var(--r-pill);display:flex;align-items:center;justify-content:center;flex-shrink:0;
    font:800 12px/1 var(--ui);width:38px;height:38px;position:relative;transition:.3s var(--ease)}
  .av:hover{transform:scale(1.08)}
  .av.a{background:var(--chip-bg);color:var(--agent-ink);border:1px solid var(--hair);
    box-shadow:inset 0 1px 0 var(--hair)}
  .av.p{color:var(--pri-ink);background:var(--pri-grad);box-shadow:var(--pri-sh);
    animation:breathe 5s var(--ease) infinite}
  .av.h{color:var(--me-ink);background:var(--me-grad);box-shadow:var(--me-sh)}
  .av.sm{width:30px;height:30px;font-size:10.5px}
  .av.xs{width:23px;height:23px;font-size:9px}
  .st{position:absolute;right:-1px;bottom:-1px;width:11px;height:11px;border-radius:var(--r-pill);
    border:2.5px solid rgba(255,255,255,.9)}
  .dark .st{border-color:#0a0c12}
  .st.on{background:#22d69a;box-shadow:0 0 12px #22d69a;animation:pulse 2.8s var(--ease) infinite}
  .st.off{background:var(--ink3)}
  @keyframes breathe{0%,100%{transform:scale(1)}50%{transform:scale(1.05)}}
  @keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:.55;transform:scale(.84)}}

  .chip{display:inline-flex;align-items:center;gap:6px;padding:6px 13px;border-radius:var(--r-pill);
    font:700 11px/1 var(--ui);background:var(--chip-bg);color:var(--ink2);white-space:nowrap;
    border:1px solid var(--hair);box-shadow:inset 0 1px 0 var(--hair);transition:.26s var(--ease)}
  .chip:hover{color:var(--ink);transform:translateY(-1px)}
  .chip.a{background:var(--agent-soft);color:var(--agent-ink)}
  .chip.h{background:var(--human-soft);color:var(--human-ink)}
  .chip.w{background:var(--warn-soft);color:var(--warn)}
  .chip.al{background:var(--alert-soft);color:var(--alert)}
  .chip.solid{background:var(--ink);color:#fff;border-color:transparent}
  .btn{display:inline-flex;align-items:center;gap:9px;padding:11px 20px;border-radius:var(--r-pill);
    font:700 12.5px/1 var(--ui);background:var(--chip-bg);color:var(--ink);
    backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
    border:1px solid var(--hair);box-shadow:inset 0 1px 0 var(--hair),0 6px 16px -10px rgba(20,55,85,.4);
    transition:.3s var(--ease)}
  .btn:hover{transform:translateY(-2px)}
  .btn:active{transform:translateY(0) scale(.98)}
  .btn.pri{color:var(--pri-ink);border-color:transparent;background:var(--pri-grad);box-shadow:var(--pri-sh)}
  .btn.pri:hover{box-shadow:var(--pri-sh),0 0 42px -8px rgba(90,220,200,.7)}
  .btn.gh{background:transparent;box-shadow:none;border-color:transparent;color:var(--ink2)}
  .btn.gh:hover{background:var(--chip-bg);color:var(--ink)}
  .ico{width:46px;height:46px;padding:0;justify-content:center;border-radius:var(--r-pill)}

  .card{border-radius:var(--r-card);background:var(--chip-bg);border:1px solid var(--hair);
    box-shadow:inset 0 1px 0 var(--hair);transition:.34s var(--ease);position:relative}
  .card:hover{transform:translateY(-2px)}
  .card .hd{padding:15px 18px 0;font:800 10.5px/1 var(--ui);letter-spacing:.1em;
    color:var(--ink3);text-transform:uppercase}
  .card .bd{padding:16px 18px;display:flex;flex-direction:column;gap:12px}
  .kv{display:flex;align-items:center;gap:8px;font:600 12px/1.5 var(--ui);color:var(--ink2)}
  .kv b{margin-left:auto;color:var(--ink);font-weight:800}
  .mono{font-family:var(--mono)}
  .sep{height:1px;background:var(--hair2)}
  .lbl{font:800 10px/1 var(--ui);letter-spacing:.1em;color:var(--ink3);text-transform:uppercase}
  .in{background:var(--inset-bg);border:1px solid var(--inset-bd);border-radius:var(--r-pill);
    padding:13px 20px;font:600 13px/1.5 var(--ui);color:var(--ink);box-shadow:var(--inset-sh);
    transition:.28s var(--ease)}

  /* 语义流光：只给主 agent 与当前会话 */
  .glow::before{content:'';position:absolute;inset:0;border-radius:inherit;padding:1.4px;
    background:conic-gradient(from var(--ang),var(--i1),var(--i2),var(--i3),var(--i4),var(--i5),var(--i1));
    -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
    -webkit-mask-composite:xor;mask-composite:exclude;
    animation:spin 7s linear infinite;opacity:.9;pointer-events:none}
  .runner::after{content:'';position:absolute;inset:-1px;border-radius:inherit;padding:1.6px;
    background:conic-gradient(from var(--ang),transparent 0 64%,var(--i1) 76%,#fff 82%,var(--i3) 88%,transparent 96%);
    -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
    -webkit-mask-composite:xor;mask-composite:exclude;
    animation:spin 3.6s linear infinite;pointer-events:none}

  /* ═══ 气泡：也是厚玻璃胶囊 ═══ */
  .msg{display:flex;gap:12px;align-items:flex-end;max-width:76%;animation:rise .66s var(--ease) both}
  .msg.me{margin-left:auto;flex-direction:row-reverse}
  .bub{padding:15px 20px;border-radius:var(--r-bub);font:500 13.5px/1.7 var(--ui);
    background:var(--pane-bg);color:var(--ink);border:1px solid var(--pane-bd);
    backdrop-filter:blur(20px) saturate(180%);-webkit-backdrop-filter:blur(20px) saturate(180%);
    box-shadow:inset 0 1.5px 0 var(--hair),inset 0 -16px 30px -24px rgba(60,115,150,.4),
               0 14px 32px -18px rgba(20,55,85,.36);
    border-bottom-left-radius:9px;transition:.34s var(--ease)}
  .dark .bub{box-shadow:inset 0 1px 0 rgba(255,255,255,.13),0 18px 40px -22px #000}
  .bub:hover{transform:translateY(-2px)}
  .bub.me{color:var(--me-ink);border-color:transparent;background:var(--me-grad);
    border-bottom-left-radius:var(--r-bub);border-bottom-right-radius:9px;box-shadow:var(--me-sh)}
  .bub.pri{border-color:color-mix(in srgb,var(--agent) 45%,transparent);
    box-shadow:inset 0 1.5px 0 var(--hair),0 16px 38px -20px color-mix(in srgb,var(--agent) 55%,transparent)}
  .bub.watch{background:transparent;border:1px dashed var(--hair2);box-shadow:none;color:var(--ink2);
    backdrop-filter:none;-webkit-backdrop-filter:none}
  .who{display:flex;align-items:center;gap:8px;margin-bottom:8px;font:800 11.5px/1 var(--ui)}
  .who .t{margin-left:6px;font:600 10.5px/1 var(--ui);color:var(--ink3)}
  .msg.me .who{justify-content:flex-end}
  .sys{align-self:center;padding:8px 17px;border-radius:var(--r-pill);background:var(--chip-bg);
    border:1px solid var(--hair);box-shadow:inset 0 1px 0 var(--hair);
    font:700 11px/1 var(--ui);color:var(--ink3);backdrop-filter:blur(14px);
    animation:rise .66s var(--ease) both}
  .at{color:var(--agent-ink);font-weight:800}
  .bub.me .at{color:#7a3405}
  @keyframes rise{from{opacity:0;transform:translateY(18px)}to{opacity:1;transform:none}}

  .stream > *:nth-child(1){animation-delay:.02s} .stream > *:nth-child(2){animation-delay:.09s}
  .stream > *:nth-child(3){animation-delay:.16s} .stream > *:nth-child(4){animation-delay:.23s}
  .stream > *:nth-child(5){animation-delay:.30s} .stream > *:nth-child(6){animation-delay:.37s}
  .stream > *:nth-child(7){animation-delay:.44s} .stream > *:nth-child(8){animation-delay:.51s}
  .stream > *:nth-child(9){animation-delay:.58s} .stream > *:nth-child(n+10){animation-delay:.65s}
  .card,.convo{animation:rise .72s var(--ease) both}
  .aside > *:nth-child(2){animation-delay:.1s} .aside > *:nth-child(3){animation-delay:.18s}
  .aside > *:nth-child(4){animation-delay:.26s}
  @supports (animation-timeline: view()){
    .reveal > *{animation:rise linear both;animation-timeline:view();animation-range:entry 0% cover 26%}}
  @media (prefers-reduced-motion: reduce){*,*::before,*::after{animation:none!important;transition:none!important}}

  /* ═══ 3D 球体 ═══ */
  .orb-wrap{position:relative;width:300px;height:300px;perspective:900px}
  .orb{position:absolute;inset:18px;border-radius:50%;transform-style:preserve-3d;
    background:
      radial-gradient(circle at 32% 26%,rgba(255,255,255,.95),rgba(255,255,255,0) 36%),
      radial-gradient(circle at 72% 78%,color-mix(in srgb,var(--i3) 68%,transparent),transparent 54%),
      conic-gradient(from 210deg,var(--i1),var(--i2),var(--i3),var(--i4),var(--i5),var(--i1));
    box-shadow:inset -22px -28px 66px rgba(10,40,60,.5),inset 16px 18px 46px rgba(255,255,255,.5),
      0 0 100px -10px rgba(120,220,255,.55),0 34px 74px -30px rgba(20,55,85,.5);
    animation:orbspin 24s linear infinite,breathe 6.5s var(--ease) infinite}
  .orb::after{content:'';position:absolute;inset:-38px;border-radius:50%;
    background:radial-gradient(circle,rgba(120,230,220,.5),transparent 62%);filter:blur(28px);z-index:-1}
  .ring{position:absolute;inset:0;border-radius:50%;border:1.5px solid transparent;
    border-top-color:var(--i1);border-right-color:var(--i3);
    transform:rotateX(74deg);animation:ringspin 10s linear infinite}
  .ring.b{transform:rotateX(74deg) rotateZ(60deg);border-top-color:var(--i2);
    border-right-color:var(--i4);animation-duration:14s;animation-direction:reverse}
  @keyframes orbspin{to{filter:hue-rotate(360deg)}}
  @keyframes ringspin{to{transform:rotateX(74deg) rotateZ(360deg)}}
  </style>'''
