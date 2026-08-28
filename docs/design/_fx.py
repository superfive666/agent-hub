# -*- coding: utf-8 -*-
import random
random.seed(20260828)

def particles(n, spread=(1600,1400)):
    """用单个伪元素的多重 box-shadow 画粒子场，零额外标签"""
    cols=['rgba(120,232,255,%.2f)','rgba(160,140,255,%.2f)','rgba(255,140,215,%.2f)','rgba(255,200,140,%.2f)']
    out=[]
    for _ in range(n):
        x=random.randint(0,spread[0]); y=random.randint(0,spread[1])
        a=round(random.uniform(.18,.72),2)
        c=random.choice(cols)%a
        blur=random.choice([0,0,0,1,2])
        out.append('%dpx %dpx %dpx %s'%(x,y,blur,c))
    return ','.join(out)

P_SMALL = particles(90)
P_BIG   = particles(26)

STYLE = '''  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap">
  <style>
  @property --ang { syntax:'<angle>'; initial-value:0deg; inherits:false; }

  /* ══ 亮色：冷调雾面玻璃 ══ */
  :root{
    --bg:#e9edf6; --bg2:#dfe5f2;
    --panel:rgba(255,255,255,.52); --surface:rgba(255,255,255,.72); --raised:rgba(255,255,255,.5);
    --text:#161b26; --text2:#535d72; --text3:#8a93a7;
    --line:rgba(22,27,38,.09); --line2:rgba(22,27,38,.055);
    --hair:rgba(255,255,255,.85);
    --agent:#0c9e8d; --agent-ink:#07786b; --agent-soft:rgba(12,158,141,.13); --agent-glow:rgba(12,200,180,.30);
    --human:#dd6f22; --human-ink:#a8500f; --human-soft:rgba(221,111,34,.14); --human-glow:rgba(255,150,70,.32);
    --alert:#c9412c; --alert-soft:rgba(201,65,44,.12);
    --warn:#9a7412;  --warn-soft:rgba(154,116,18,.13);
    --i1:#28c8e8; --i2:#7a6bff; --i3:#ff5cc8; --i4:#ffab4d;
    --glass-blur:22px; --glass-sat:170%;
    --shadow:0 1px 0 rgba(255,255,255,.7) inset, 0 18px 44px -22px rgba(22,30,55,.4);
    --shadow-sm:0 1px 0 rgba(255,255,255,.6) inset, 0 6px 18px -12px rgba(22,30,55,.34);
    --amb1:rgba(80,200,255,.55); --amb2:rgba(150,120,255,.45);
    --amb3:rgba(255,130,200,.36); --amb4:rgba(255,190,120,.36);
    --amb-blend:normal; --amb-op:.55; --dust-op:.35;
  }
  /* ══ 暗色：赛博霓虹 ══ */
  .dark{
    --bg:#080a11; --bg2:#0d1018;
    --panel:rgba(255,255,255,.035); --surface:rgba(255,255,255,.052); --raised:rgba(255,255,255,.075);
    --text:#e9edf8; --text2:#98a2ba; --text3:#5f6a84;
    --line:rgba(255,255,255,.09); --line2:rgba(255,255,255,.055);
    --hair:rgba(255,255,255,.12);
    --agent:#3ee0c8; --agent-ink:#7ff0de; --agent-soft:rgba(62,224,200,.13); --agent-glow:rgba(62,224,200,.42);
    --human:#ff9f5a; --human-ink:#ffc08f; --human-soft:rgba(255,159,90,.14); --human-glow:rgba(255,159,90,.42);
    --alert:#ff6f5c; --alert-soft:rgba(255,111,92,.13);
    --warn:#f0c25a;  --warn-soft:rgba(240,194,90,.12);
    --i1:#41e0ff; --i2:#8b7bff; --i3:#ff5cc8; --i4:#ffb35c;
    --glass-blur:26px; --glass-sat:150%;
    --shadow:0 1px 0 rgba(255,255,255,.08) inset, 0 26px 60px -28px rgba(0,0,0,.95);
    --shadow-sm:0 1px 0 rgba(255,255,255,.06) inset, 0 10px 26px -16px rgba(0,0,0,.9);
    --amb1:rgba(30,160,255,.55); --amb2:rgba(130,70,255,.5);
    --amb3:rgba(255,60,180,.38); --amb4:rgba(0,220,190,.35);
    --amb-blend:screen; --amb-op:.85; --dust-op:1;
  }
  :root,.dark{
    --r-bub:20px; --r-card:18px; --r-ctl:12px; --r-pill:999px;
    --ui:'Manrope',-apple-system,'PingFang SC','Noto Sans SC','Microsoft YaHei',sans-serif;
    --mono:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;
    --ease:cubic-bezier(.22,.61,.28,1);
  }
  *{box-sizing:border-box}
  body{margin:0;font-family:var(--ui);-webkit-font-smoothing:antialiased;text-wrap:pretty}
  a{color:var(--human)}

  /* ══ 底层：着色器渐变 + 粒子场（纯 CSS，无额外标签） ══ */
  .app{display:flex;background:var(--bg);color:var(--text);position:relative;isolation:isolate;overflow:hidden}
  .app::before{content:'';position:absolute;inset:-15%;z-index:0;pointer-events:none;
    opacity:var(--amb-op);mix-blend-mode:var(--amb-blend);
    background:
      radial-gradient(38% 32% at 14% 12%, var(--amb1), transparent 70%),
      radial-gradient(34% 30% at 84% 20%, var(--amb2), transparent 70%),
      radial-gradient(40% 34% at 72% 88%, var(--amb3), transparent 70%),
      radial-gradient(30% 26% at 24% 82%, var(--amb4), transparent 70%);
    filter:blur(58px);animation:drift 26s var(--ease) infinite alternate}
  .app::after{content:'';position:absolute;left:0;top:0;width:2px;height:2px;border-radius:50%;
    z-index:0;pointer-events:none;opacity:var(--dust-op);
    box-shadow:''' + P_SMALL + ''';animation:dust 34s linear infinite}
  .app > *{position:relative;z-index:1}
  @keyframes drift{
    0%{transform:translate3d(0,0,0) scale(1)}
    50%{transform:translate3d(2.5%,-2%,0) scale(1.07)}
    100%{transform:translate3d(-2%,2.5%,0) scale(1.03)}}
  @keyframes dust{from{transform:translate3d(0,0,0)}to{transform:translate3d(-40px,-90px,0)}}

  /* ══ 玻璃基元 ══ */
  .glass{background:var(--surface);backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
         -webkit-backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
         border:1px solid var(--line);box-shadow:var(--shadow)}
  /* 流光边框：只给有语义的元素（主 agent、当前会话、主按钮） */
  .glow{position:relative}
  .glow::before{content:'';position:absolute;inset:0;border-radius:inherit;padding:1.2px;
    background:conic-gradient(from var(--ang),var(--i1),var(--i2),var(--i3),var(--i4),var(--agent),var(--i1));
    -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
    -webkit-mask-composite:xor;mask-composite:exclude;
    animation:spin 7s linear infinite;opacity:.85;pointer-events:none}
  /* 边缘流光：一道亮带沿着边跑 */
  .runner::after{content:'';position:absolute;inset:-1px;border-radius:inherit;padding:1.4px;
    background:conic-gradient(from var(--ang),transparent 0 62%,var(--i1) 74%,#fff 80%,var(--i3) 86%,transparent 96%);
    -webkit-mask:linear-gradient(#000 0 0) content-box,linear-gradient(#000 0 0);
    -webkit-mask-composite:xor;mask-composite:exclude;
    animation:spin 3.4s linear infinite;pointer-events:none}
  @keyframes spin{to{--ang:360deg}}

  /* ══ 骨架 ══ */
  .rail{width:264px;flex-shrink:0;display:flex;flex-direction:column;
        background:var(--panel);backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
        -webkit-backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
        border-right:1px solid var(--line)}
  .brand{padding:20px 18px 14px;display:flex;align-items:center;gap:10px}
  .brand .mk{width:32px;height:32px;border-radius:11px;display:flex;align-items:center;justify-content:center;
    font:800 13px/1 var(--ui);color:#04121a;
    background:linear-gradient(140deg,var(--i1),var(--agent) 45%,var(--i2));
    box-shadow:0 0 22px var(--agent-glow);animation:breathe 4.5s var(--ease) infinite}
  .brand b{font:800 15px/1 var(--ui);letter-spacing:-.02em}
  .seg{display:flex;gap:3px;margin:0 14px 12px;padding:3px;border-radius:var(--r-ctl);
       background:var(--raised);border:1px solid var(--line2)}
  .seg span{flex:1;text-align:center;padding:7px 0;border-radius:9px;font:600 12px/1 var(--ui);
            color:var(--text2);transition:.28s var(--ease)}
  .seg span:hover{color:var(--text)}
  .seg span.on{background:var(--surface);color:var(--text);box-shadow:var(--shadow-sm);
               border:1px solid var(--hair)}
  .convo{display:flex;gap:11px;padding:11px 13px;margin:0 8px;border-radius:var(--r-ctl);
         align-items:flex-start;transition:.3s var(--ease)}
  .convo:hover{background:var(--raised);transform:translateX(2px)}
  .convo.on{background:var(--surface);box-shadow:var(--shadow-sm)}
  .convo .tt{font:600 12.5px/1.35 var(--ui);display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .convo .pv{font:400 11px/1.4 var(--ui);color:var(--text3);display:block;overflow:hidden;
             text-overflow:ellipsis;white-space:nowrap;margin-top:3px}
  .railfoot{margin-top:auto;padding:12px 14px;display:flex;flex-direction:column;gap:9px}
  .main{flex-grow:1;display:flex;flex-direction:column;min-width:0}
  .head{padding:16px 24px;border-bottom:1px solid var(--line);display:flex;align-items:center;gap:14px;
        background:var(--panel);backdrop-filter:blur(var(--glass-blur));
        -webkit-backdrop-filter:blur(var(--glass-blur))}
  .head h1{margin:0;font:800 18px/1.25 var(--ui);letter-spacing:-.025em}
  .head .sub{font:500 11.5px/1.4 var(--ui);color:var(--text3);margin-top:5px}
  .head .sp{margin-left:auto;display:flex;gap:8px;align-items:center}
  .body{flex-grow:1;display:flex;min-width:0}
  .stream{flex-grow:1;padding:22px 24px;display:flex;flex-direction:column;gap:16px;min-width:0}
  .aside{width:290px;flex-shrink:0;border-left:1px solid var(--line);padding:20px;
         display:flex;flex-direction:column;gap:14px;background:var(--panel);
         backdrop-filter:blur(var(--glass-blur));-webkit-backdrop-filter:blur(var(--glass-blur))}

  /* ══ 原子 ══ */
  .av{border-radius:var(--r-pill);display:flex;align-items:center;justify-content:center;flex-shrink:0;
      font:700 12px/1 var(--ui);width:36px;height:36px;position:relative;transition:.3s var(--ease)}
  .av:hover{transform:scale(1.07)}
  .av.a{background:var(--agent-soft);color:var(--agent-ink);border:1px solid var(--line)}
  .av.p{color:#04121a;background:linear-gradient(140deg,var(--i1),var(--agent) 50%,var(--i2));
        box-shadow:0 0 0 3px color-mix(in srgb,var(--agent) 26%,transparent),0 0 26px var(--agent-glow);
        animation:breathe 4.5s var(--ease) infinite}
  .av.h{color:#22120a;background:linear-gradient(140deg,var(--i4),var(--human));
        box-shadow:0 0 20px var(--human-glow)}
  .av.sm{width:28px;height:28px;font-size:10.5px}
  .av.xs{width:22px;height:22px;font-size:9px}
  .st{position:absolute;right:-1px;bottom:-1px;width:11px;height:11px;border-radius:var(--r-pill);
      border:2.5px solid var(--bg)}
  .st.on{background:#37e39f;box-shadow:0 0 10px #37e39f;animation:pulse 2.6s var(--ease) infinite}
  .st.off{background:var(--text3)}
  @keyframes breathe{0%,100%{transform:scale(1)}50%{transform:scale(1.045)}}
  @keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:.6;transform:scale(.86)}}

  .chip{display:inline-flex;align-items:center;gap:5px;padding:4px 10px;border-radius:var(--r-pill);
        font:600 11px/1 var(--ui);background:var(--raised);color:var(--text2);white-space:nowrap;
        border:1px solid var(--line2);transition:.25s var(--ease)}
  .chip:hover{color:var(--text);border-color:var(--line)}
  .chip.a{background:var(--agent-soft);color:var(--agent-ink);border-color:color-mix(in srgb,var(--agent) 26%,transparent)}
  .chip.h{background:var(--human-soft);color:var(--human-ink);border-color:color-mix(in srgb,var(--human) 30%,transparent)}
  .chip.w{background:var(--warn-soft);color:var(--warn);border-color:color-mix(in srgb,var(--warn) 26%,transparent)}
  .chip.al{background:var(--alert-soft);color:var(--alert);border-color:color-mix(in srgb,var(--alert) 30%,transparent)}
  .chip.solid{background:var(--text);color:var(--bg)}
  .btn{display:inline-flex;align-items:center;gap:7px;padding:9px 16px;border-radius:var(--r-ctl);
       font:600 12.5px/1 var(--ui);background:var(--surface);color:var(--text);
       backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);
       border:1px solid var(--line);box-shadow:var(--shadow-sm);transition:.28s var(--ease)}
  .btn:hover{transform:translateY(-1.5px);border-color:var(--hair)}
  .btn:active{transform:translateY(0) scale(.985)}
  .btn.pri{color:#04121a;border-color:transparent;
    background:linear-gradient(135deg,var(--i1),var(--agent) 55%,var(--i2));
    box-shadow:0 0 26px var(--agent-glow),0 1px 0 rgba(255,255,255,.28) inset}
  .btn.pri:hover{box-shadow:0 0 40px var(--agent-glow),0 1px 0 rgba(255,255,255,.34) inset}
  .btn.gh{background:transparent;box-shadow:none;border-color:transparent;color:var(--text2)}
  .btn.gh:hover{background:var(--raised);color:var(--text)}

  .card{border-radius:var(--r-card);background:var(--surface);
        backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
        -webkit-backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
        border:1px solid var(--line);box-shadow:var(--shadow);transition:.34s var(--ease)}
  .card:hover{transform:translateY(-2px);border-color:var(--hair)}
  .card .hd{padding:14px 17px 0;font:700 11px/1 var(--ui);letter-spacing:.08em;
            color:var(--text3);text-transform:uppercase}
  .card .bd{padding:15px 17px;display:flex;flex-direction:column;gap:11px}
  .kv{display:flex;align-items:center;gap:8px;font:500 12px/1.5 var(--ui);color:var(--text2)}
  .kv b{margin-left:auto;color:var(--text);font-weight:700}
  .mono{font-family:var(--mono)}
  .sep{height:1px;background:var(--line2)}
  .lbl{font:700 10.5px/1 var(--ui);letter-spacing:.08em;color:var(--text3);text-transform:uppercase}
  .in{background:var(--raised);border:1px solid var(--line);border-radius:var(--r-ctl);
      padding:11px 14px;font:500 13px/1.5 var(--ui);color:var(--text);transition:.28s var(--ease)}
  .in:hover{border-color:var(--hair)}

  /* ══ 气泡 ══ */
  .msg{display:flex;gap:11px;align-items:flex-end;max-width:74%;
       animation:rise .62s var(--ease) both}
  .msg.me{margin-left:auto;flex-direction:row-reverse}
  .bub{padding:13px 17px;border-radius:var(--r-bub);font:400 13.5px/1.68 var(--ui);
       background:var(--surface);color:var(--text);border:1px solid var(--line);
       backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
       -webkit-backdrop-filter:blur(var(--glass-blur)) saturate(var(--glass-sat));
       box-shadow:var(--shadow-sm);border-bottom-left-radius:7px;transition:.32s var(--ease)}
  .bub:hover{transform:translateY(-2px);border-color:var(--hair)}
  .bub.me{color:#22120a;border-color:transparent;border-bottom-left-radius:var(--r-bub);
          border-bottom-right-radius:7px;
          background:linear-gradient(135deg,var(--i4),var(--human));
          box-shadow:0 0 32px var(--human-glow),0 1px 0 rgba(255,255,255,.3) inset}
  .bub.pri{border-color:color-mix(in srgb,var(--agent) 42%,transparent);
           box-shadow:var(--shadow-sm),0 0 26px color-mix(in srgb,var(--agent) 20%,transparent)}
  .bub.watch{background:transparent;border:1px dashed var(--line);box-shadow:none;color:var(--text2);
             backdrop-filter:none;-webkit-backdrop-filter:none}
  .who{display:flex;align-items:center;gap:7px;margin-bottom:7px;font:700 11.5px/1 var(--ui)}
  .who .t{margin-left:6px;font:500 10.5px/1 var(--ui);color:var(--text3)}
  .msg.me .who{justify-content:flex-end}
  .sys{align-self:center;padding:7px 15px;border-radius:var(--r-pill);background:var(--raised);
       border:1px solid var(--line2);font:600 11px/1 var(--ui);color:var(--text3);
       backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);
       animation:rise .62s var(--ease) both}
  .at{color:var(--agent-ink);font-weight:800}
  .bub.me .at{color:#5c2c06}
  @keyframes rise{from{opacity:0;transform:translateY(16px)}to{opacity:1;transform:none}}

  /* 入场错峰：整页“活”起来 */
  .stream > *:nth-child(1){animation-delay:.02s}
  .stream > *:nth-child(2){animation-delay:.09s}
  .stream > *:nth-child(3){animation-delay:.16s}
  .stream > *:nth-child(4){animation-delay:.23s}
  .stream > *:nth-child(5){animation-delay:.30s}
  .stream > *:nth-child(6){animation-delay:.37s}
  .stream > *:nth-child(7){animation-delay:.44s}
  .stream > *:nth-child(8){animation-delay:.51s}
  .stream > *:nth-child(9){animation-delay:.58s}
  .stream > *:nth-child(10){animation-delay:.65s}
  .stream > *:nth-child(n+11){animation-delay:.72s}
  .card,.convo{animation:rise .7s var(--ease) both}
  .aside > *:nth-child(2){animation-delay:.1s}
  .aside > *:nth-child(3){animation-delay:.18s}
  .aside > *:nth-child(4){animation-delay:.26s}

  /* 滚动显现：支持才启用，不支持也不会把内容藏起来 */
  @supports (animation-timeline: view()){
    .reveal > *{animation:rise linear both;animation-timeline:view();animation-range:entry 0% cover 26%}
  }
  /* 尊重减弱动效 */
  @media (prefers-reduced-motion: reduce){
    *,*::before,*::after{animation:none!important;transition:none!important}
  }

  /* ══ 3D 球体 ══ */
  .orb-wrap{position:relative;width:300px;height:300px;perspective:900px}
  .orb{position:absolute;inset:18px;border-radius:50%;transform-style:preserve-3d;
    background:
      radial-gradient(circle at 33% 27%,rgba(255,255,255,.92),rgba(255,255,255,0) 34%),
      radial-gradient(circle at 72% 76%,color-mix(in srgb,var(--i3) 70%,transparent),transparent 52%),
      conic-gradient(from 210deg,var(--i1),var(--i2),var(--i3),var(--i4),var(--agent),var(--i1));
    box-shadow:inset -20px -26px 62px rgba(3,5,12,.72),inset 14px 16px 44px rgba(255,255,255,.14),
               0 0 90px var(--agent-glow),0 30px 70px -28px rgba(0,0,0,.8);
    animation:orbspin 22s linear infinite,breathe 6s var(--ease) infinite}
  .orb::after{content:'';position:absolute;inset:-34px;border-radius:50%;
    background:radial-gradient(circle,var(--agent-glow),transparent 62%);filter:blur(26px);z-index:-1}
  .ring{position:absolute;inset:0;border-radius:50%;border:1.5px solid transparent;
    border-top-color:var(--i1);border-right-color:var(--i3);
    transform:rotateX(74deg);animation:ringspin 9s linear infinite}
  .ring.b{transform:rotateX(74deg) rotateZ(60deg);border-top-color:var(--i2);
          border-right-color:var(--i4);animation-duration:13s;animation-direction:reverse}
  @keyframes orbspin{to{filter:hue-rotate(360deg)}}
  @keyframes ringspin{to{transform:rotateX(74deg) rotateZ(360deg)}}
  </style>'''
