/**
 * LibreSpeed — modern UI with canvas gauges
 */

const INITIALIZING = 0;
const READY = 1;
const RUNNING = 2;
const FINISHED = 3;

const appState = {
  ui: INITIALIZING,
  speedtest: null,
  servers: [],
  selectedServerDirty: false,
  data: null,
  dataDirty: false,
  telemetryEnabled: false,
};

// ── Gauge geometry ──────────────────────────────────────────────────────────
// 240° arc, open at bottom (lower-left → top → lower-right, clockwise)
const G_START = (5 * Math.PI) / 6;   // 150° — lower-left
const G_END   = Math.PI / 6;          // 30°  — lower-right
const G_SWEEP = (4 * Math.PI) / 3;    // 240°

function valueToAngle(value, maxValue, isLog) {
  let r = isLog
    ? Math.log10(Math.max(0.001, value) + 1) / Math.log10(maxValue + 1)
    : value / maxValue;
  r = Math.max(0, Math.min(1, r));
  return G_START + r * G_SWEEP;
}

// ── Tick builders ───────────────────────────────────────────────────────────
function buildLogTicks() {
  const ticks = [];
  // major labeled
  [1, 10, 100, 1000, 10000].forEach(v => {
    ticks.push({ v, label: v >= 1000 ? (v / 1000) + 'G' : String(v), major: true });
  });
  // minor
  [2,3,4,5,6,7,8,9,
   20,30,40,50,60,70,80,90,
   200,300,400,500,600,700,800,900,
   2000,3000,4000,5000,6000,7000,8000,9000].forEach(v => {
    ticks.push({ v, major: false });
  });
  return ticks;
}

function buildLinearTicks(max, minorVals, majorVals) {
  const map = new Map();
  minorVals.forEach(v => map.set(v, { v, major: false }));
  majorVals.forEach(v => map.set(v, { v, label: String(v), major: true }));
  return Array.from(map.values());
}

// ── Gauge config ────────────────────────────────────────────────────────────
const GAUGES = {
  dl: {
    color: '#22d3ee', glow: 'rgba(34,211,238,0.35)',
    label: 'DOWNLOAD', unit: 'Mbps',
    isLog: true, maxValue: 10000,
    ticks: buildLogTicks(),
    canvas: null,
  },
  ul: {
    color: '#a78bfa', glow: 'rgba(167,139,250,0.35)',
    label: 'UPLOAD', unit: 'Mbps',
    isLog: true, maxValue: 10000,
    ticks: buildLogTicks(),
    canvas: null,
  },
  ping: {
    color: '#34d399', glow: 'rgba(52,211,153,0.3)',
    label: 'PING', unit: 'ms',
    isLog: false, maxValue: 500,
    ticks: buildLinearTicks(500,
      [0, 50, 100, 150, 200, 250, 300, 400, 500],
      [0, 100, 200, 300, 500]),
    canvas: null,
  },
  jitter: {
    color: '#fbbf24', glow: 'rgba(251,191,36,0.3)',
    label: 'JITTER', unit: 'ms',
    isLog: false, maxValue: 150,
    ticks: buildLinearTicks(150,
      [0, 25, 50, 75, 100, 125, 150],
      [0, 50, 100, 150]),
    canvas: null,
  },
};

// ── Core draw function ──────────────────────────────────────────────────────
function drawGauge(cfg, value, progress, active, dimmed) {
  const canvas = cfg.canvas;
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const W = canvas.clientWidth * dpr;
  const H = canvas.clientHeight * dpr;
  if (W === 0 || H === 0) return;
  if (canvas.width !== W || canvas.height !== H) {
    canvas.width = W; canvas.height = H;
  }
  ctx.clearRect(0, 0, W, H);

  // geometry
  const cx = W / 2;
  const R  = Math.min(W * 0.36, H * 0.55);
  const cy = R + H * 0.08;   // arc center — top portion
  const tw = R * 0.09;        // track width

  const alpha = dimmed ? 0.35 : 1;

  // ── background track
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.beginPath();
  ctx.arc(cx, cy, R, G_START, G_END, false);
  ctx.strokeStyle = 'rgba(255,255,255,0.07)';
  ctx.lineWidth = tw;
  ctx.lineCap = 'round';
  ctx.stroke();

  // ── tick marks
  cfg.ticks.forEach(({ v, label, major }) => {
    const a = valueToAngle(v, cfg.maxValue, cfg.isLog);
    const cos = Math.cos(a), sin = Math.sin(a);
    const outerR = R + tw * 0.15;
    const innerR = major ? R - tw * 0.9 : R - tw * 0.45;

    ctx.beginPath();
    ctx.moveTo(cx + outerR * cos, cy + outerR * sin);
    ctx.lineTo(cx + innerR * cos, cy + innerR * sin);
    ctx.strokeStyle = major ? 'rgba(255,255,255,0.35)' : 'rgba(255,255,255,0.12)';
    ctx.lineWidth = major ? 1.5 * dpr : 0.8 * dpr;
    ctx.lineCap = 'butt';
    ctx.stroke();

    if (label && major) {
      const lr = R - tw * 1.75;
      ctx.font = `${Math.round(9.5 * dpr)}px Inter,sans-serif`;
      ctx.fillStyle = 'rgba(255,255,255,0.4)';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(label, cx + lr * cos, cy + lr * sin);
    }
  });
  ctx.restore();

  // ── value arc + glow
  if (value > 0) {
    const va = valueToAngle(value, cfg.maxValue, cfg.isLog);

    if (active) {
      ctx.save();
      ctx.globalAlpha = 0.45;
      ctx.beginPath();
      ctx.arc(cx, cy, R, G_START, va, false);
      ctx.strokeStyle = cfg.color;
      ctx.lineWidth = tw * 2.8;
      ctx.lineCap = 'round';
      ctx.filter = `blur(${tw * 0.8}px)`;
      ctx.stroke();
      ctx.restore();
    }

    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.beginPath();
    ctx.arc(cx, cy, R, G_START, va, false);
    ctx.strokeStyle = cfg.color;
    ctx.lineWidth = tw;
    ctx.lineCap = 'round';
    if (active) {
      ctx.shadowColor = cfg.color;
      ctx.shadowBlur = 10 * dpr;
    }
    ctx.stroke();
    ctx.restore();

    // tip dot
    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.beginPath();
    ctx.arc(cx + R * Math.cos(va), cy + R * Math.sin(va), tw * 0.65, 0, 2 * Math.PI);
    ctx.fillStyle = '#fff';
    ctx.shadowColor = cfg.color;
    ctx.shadowBlur = active ? 14 * dpr : 6 * dpr;
    ctx.fill();
    ctx.restore();
  }

  // ── progress ring (thin outer arc)
  if (progress > 0 && progress < 1) {
    const pa = G_START + progress * G_SWEEP;
    ctx.save();
    ctx.globalAlpha = 0.5;
    ctx.beginPath();
    ctx.arc(cx, cy, R + tw * 1.05, G_START, pa, false);
    ctx.strokeStyle = cfg.color;
    ctx.lineWidth = 2 * dpr;
    ctx.lineCap = 'round';
    ctx.stroke();
    ctx.restore();
  }

  // ── value text (inside bowl, below arc center)
  const textAlpha = dimmed ? 0.25 : (value > 0 ? 1 : 0.2);
  const displayVal = value <= 0 ? '–' : numberToText(value);

  // main number
  const numSz = Math.round(R * 0.38);
  ctx.save();
  ctx.globalAlpha = textAlpha;
  ctx.font = `200 ${numSz}px Inter,sans-serif`;
  ctx.fillStyle = value > 0 ? '#fff' : 'rgba(255,255,255,0.3)';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'alphabetic';
  const numY = cy + R * 0.28;
  ctx.fillText(displayVal, cx, numY);
  ctx.restore();

  // unit
  const unitSz = Math.round(R * 0.13);
  ctx.save();
  ctx.globalAlpha = dimmed ? 0.2 : 0.75;
  ctx.font = `500 ${unitSz}px Inter,sans-serif`;
  ctx.fillStyle = cfg.color;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillText(cfg.unit, cx, numY + unitSz * 0.3);
  ctx.restore();

  // label
  const lblSz = Math.round(R * 0.105);
  ctx.save();
  ctx.globalAlpha = dimmed ? 0.2 : 0.45;
  ctx.font = `700 ${lblSz}px Inter,sans-serif`;
  ctx.fillStyle = '#fff';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillText(cfg.label, cx, numY + unitSz * 0.3 + unitSz * 1.5);
  ctx.restore();
}

// ── Number formatter ────────────────────────────────────────────────────────
function numberToText(v) {
  v = Number(v);
  if (!v || isNaN(v)) return '0.00';
  if (v < 10)  return v.toFixed(2);
  if (v < 100) return v.toFixed(1);
  return v.toFixed(0);
}

// ── Bootstrap ───────────────────────────────────────────────────────────────
window.addEventListener('DOMContentLoaded', () => {
  GAUGES.dl.canvas     = document.getElementById('dl-gauge');
  GAUGES.ul.canvas     = document.getElementById('ul-gauge');
  GAUGES.ping.canvas   = document.getElementById('ping-gauge');
  GAUGES.jitter.canvas = document.getElementById('jitter-gauge');

  // draw initial empty state
  Object.values(GAUGES).forEach(g => drawGauge(g, 0, 0, false, false));

  createSpeedtest();
  hookUpButtons();
  startRenderLoop();
  applySettingsJSON();
  applyServerListJSON();
});

function createSpeedtest() {
  appState.speedtest = new Speedtest();
  appState.speedtest.onupdate = data => {
    appState.data = data;
    appState.dataDirty = true;
  };
  appState.speedtest.onend = aborted => {
    appState.ui = aborted ? READY : FINISHED;
  };
}

function hookUpButtons() {
  document.getElementById('start-button').addEventListener('click', () => {
    if (appState.ui === READY || appState.ui === FINISHED) {
      document.getElementById('results-panel').classList.add('hidden');
      document.getElementById('share-results').classList.add('hidden');
      appState.speedtest.start();
      appState.ui = RUNNING;
    } else if (appState.ui === RUNNING) {
      appState.speedtest.abort();
    }
  });

  document.getElementById('choose-privacy')
    ?.addEventListener('click', () => document.getElementById('privacy').showModal());

  document.getElementById('share-results')
    ?.addEventListener('click', () => document.getElementById('share').showModal());

  document.getElementById('copy-link')
    ?.addEventListener('click', async () => {
      const link = document.querySelector('img#results')?.src;
      if (!link) return;
      await navigator.clipboard.writeText(link);
      const btn = document.getElementById('copy-link');
      btn.textContent = 'Copied!';
      setTimeout(() => btn.textContent = 'Copy link', 3000);
    });

  document.querySelectorAll('.close-dialog, #close-privacy').forEach(el =>
    el.addEventListener('click', () =>
      document.querySelectorAll('dialog').forEach(d => d.close())
    )
  );
}

async function applySettingsJSON() {
  try {
    const res  = await fetch('settings.json');
    const cfg  = await res.json();
    for (const k in cfg) {
      appState.speedtest.setParameter(k, cfg[k]);
      if (k === 'telemetry_level' && cfg[k] && !['off','disabled','false'].includes(String(cfg[k]))) {
        appState.telemetryEnabled = true;
        document.getElementById('privacy-warning')?.classList.remove('hidden');
      }
    }
  } catch (_) {}
}

async function applyServerListJSON() {
  try {
    const src = typeof globalThis.SPEEDTEST_SERVERS !== 'undefined'
      ? globalThis.SPEEDTEST_SERVERS
      : 'server-list.json';
    const servers = Array.isArray(src)
      ? src
      : await fetch(src).then(r => r.json());

    if (!servers?.length) return console.error('Server list empty');
    const server = servers[0];
    appState.speedtest.setSelectedServer(server);
    appState.selectedServerDirty = true;
    appState.ui = READY;
  } catch (e) {
    console.error('Failed to load server list', e);
  }
}

// ── Render loop ─────────────────────────────────────────────────────────────
function startRenderLoop() {
  const startBtn      = document.getElementById('start-button');
  const selectedEl    = document.getElementById('selected-server');
  const ipEl          = document.getElementById('ip-display');
  const resultsPanel  = document.getElementById('results-panel');
  const shareBtn      = document.getElementById('share-results');
  const resultsImg    = document.getElementById('results');

  const btnLabel = {
    [INITIALIZING]: 'Loading…',
    [READY]:        'Start Test',
    [RUNNING]:      'Abort',
    [FINISHED]:     'Test Again',
  };

  function render() {
    startBtn.textContent = btnLabel[appState.ui];
    startBtn.classList.toggle('disabled', appState.ui === INITIALIZING);
    startBtn.classList.toggle('active', appState.ui === RUNNING);

    if (appState.selectedServerDirty) {
      try {
        selectedEl.textContent = appState.speedtest.getSelectedServer().name;
      } catch (_) {}
      appState.selectedServerDirty = false;
    }

    if (appState.dataDirty && appState.data) {
      const d  = appState.data;
      const ts = d.testState;   // 1=dl 2=ping 3=ul
      const running = appState.ui === RUNNING;
      const done    = appState.ui === FINISHED;
      const osc = (running && ts === 1) ? 1 + 0.015 * Math.sin(Date.now() / 120) : 1;
      const oscU = (running && ts === 3) ? 1 + 0.015 * Math.sin(Date.now() / 120) : 1;

      const dlVal     = (parseFloat(d.dlStatus)     || 0) * osc;
      const ulVal     = (parseFloat(d.ulStatus)     || 0) * oscU;
      const pingVal   =  parseFloat(d.pingStatus)   || 0;
      const jitterVal =  parseFloat(d.jitterStatus) || 0;

      drawGauge(GAUGES.dl,     dlVal,     parseFloat(d.dlProgress)   || 0, ts === 1, running && ts !== 1 && !done);
      drawGauge(GAUGES.ul,     ulVal,     parseFloat(d.ulProgress)   || 0, ts === 3, running && ts !== 3 && !done);
      drawGauge(GAUGES.ping,   pingVal,   parseFloat(d.pingProgress) || 0, ts === 2, running && ts !== 2 && !done);
      drawGauge(GAUGES.jitter, jitterVal, 0,                               ts === 2, running && ts !== 2 && !done);

      // IP info
      if (d.clientIp) {
        ipEl.innerHTML = `Connected via <strong>${d.clientIp}</strong>`;
      }

      // results panel
      if (done) {
        document.getElementById('result-dl').textContent     = numberToText(d.dlStatus);
        document.getElementById('result-ul').textContent     = numberToText(d.ulStatus);
        document.getElementById('result-ping').textContent   = numberToText(d.pingStatus);
        document.getElementById('result-jitter').textContent = numberToText(d.jitterStatus);
        resultsPanel.classList.remove('hidden');

        if (appState.telemetryEnabled && d.testId) {
          shareBtn?.classList.remove('hidden');
          if (resultsImg) {
            resultsImg.src = window.location.href.replace(/[^/]*$/, '') + 'results/?id=' + d.testId;
          }
        }
      }

      appState.dataDirty = false;
    }

    requestAnimationFrame(render);
  }

  render();
}
