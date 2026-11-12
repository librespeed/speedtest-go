/**
 * Shared canvas gauge-drawing engine, used by both index.html (inline
 * bootstrap script) and index-modern.html (javascript/index.js). Keeping
 * this in one place avoids the two copies drifting apart.
 *
 * 240° arc, open at bottom (lower-left → top → lower-right, clockwise).
 */

const GAUGE_ARC_START = (5 * Math.PI) / 6; // 150°
const GAUGE_ARC_SWEEP = (4 * Math.PI) / 3; // 240°

function gaugeValueToAngle(value, max, isLog) {
  const r = isLog
    ? Math.log10(Math.max(0.001, value) + 1) / Math.log10(max + 1)
    : value / max;
  return GAUGE_ARC_START + Math.max(0, Math.min(1, r)) * GAUGE_ARC_SWEEP;
}

// Tick definitions for a log scale 1–10000 Mbps. Each decade (1-10, 10-100,
// 100-1G, 1G-10G) spans ~60° on the arc, with labels at key reference points
// including 2.5G and 5G for multi-gig connections.
function buildGaugeLogTicks() {
  const major = [
    { v: 1, label: '1' },
    { v: 5, label: '5' },
    { v: 10, label: '10' },
    { v: 50, label: '50' },
    { v: 100, label: '100' },
    { v: 500, label: '500' },
    { v: 1000, label: '1G' },
    { v: 2500, label: '2.5G' },
    { v: 5000, label: '5G' },
    { v: 10000, label: '10G' },
  ];
  const minor = [
    2, 3, 4, 6, 7, 8, 9,
    20, 30, 40, 60, 70, 80, 90,
    200, 300, 400, 600, 700, 800, 900,
    1500, 2000, 3000, 4000, 6000, 7000, 7500, 8000, 9000,
  ];
  return [
    ...major.map(m => ({ ...m, major: true })),
    ...minor.map(v => ({ v, major: false })),
  ];
}

function buildGaugeLinearTicks(vals, majors) {
  return vals.map(v => ({ v, major: majors.includes(v), label: majors.includes(v) ? '' + v : '' }));
}

// g: { color, cssVar (optional), label, unit, isLog, max, ticks }
function drawGauge(canvas, g, value, progress, active, dimmed) {
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const W = canvas.clientWidth * dpr;
  const H = canvas.clientHeight * dpr;
  if (!W || !H) return;
  if (canvas.width !== W || canvas.height !== H) { canvas.width = W; canvas.height = H; }
  ctx.clearRect(0, 0, W, H);

  // Read accent color from a CSS var when the page defines one (theme
  // switching); canvas colors can't just follow CSS, so this is re-read
  // every frame. Falls back to the static config color otherwise.
  const isLight = document.documentElement.getAttribute('data-theme') === 'light';
  const gColor = (g.cssVar && getComputedStyle(document.documentElement).getPropertyValue(g.cssVar).trim()) || g.color;
  const cTrack = isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.07)';
  const cTkMaj = isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.32)';
  const cTkMin = isLight ? 'rgba(0,0,0,0.12)' : 'rgba(255,255,255,0.10)';
  const cTkLbl = isLight ? 'rgba(0,0,0,0.45)' : 'rgba(255,255,255,0.38)';
  const cNum = isLight ? '#0f172a' : '#ffffff';

  const cx = W / 2;
  const R = Math.min(W * 0.36, H * 0.55);
  const cy = R + H * 0.08;
  const tw = R * 0.09;
  const a = dimmed ? 0.3 : 1;

  // background track
  ctx.save();
  ctx.globalAlpha = a;
  ctx.beginPath();
  ctx.arc(cx, cy, R, GAUGE_ARC_START, GAUGE_ARC_START + GAUGE_ARC_SWEEP, false);
  ctx.strokeStyle = cTrack;
  ctx.lineWidth = tw;
  ctx.lineCap = 'round';
  ctx.stroke();

  // ticks
  g.ticks.forEach(({ v, label, major }) => {
    const ang = gaugeValueToAngle(v, g.max, g.isLog);
    const cos = Math.cos(ang), sin = Math.sin(ang);
    ctx.beginPath();
    ctx.moveTo(cx + (R + tw * 0.15) * cos, cy + (R + tw * 0.15) * sin);
    ctx.lineTo(cx + (major ? R - tw * 0.9 : R - tw * 0.45) * cos, cy + (major ? R - tw * 0.9 : R - tw * 0.45) * sin);
    ctx.strokeStyle = major ? cTkMaj : cTkMin;
    ctx.lineWidth = major ? 1.5 * dpr : 0.8 * dpr;
    ctx.lineCap = 'butt';
    ctx.stroke();
    if (label && major) {
      const lr = R - tw * 1.75;
      ctx.font = `${Math.round(9.5 * dpr)}px Inter,sans-serif`;
      ctx.fillStyle = cTkLbl;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(label, cx + lr * cos, cy + lr * sin);
    }
  });
  ctx.restore();

  // value arc + glow
  if (value > 0) {
    const va = gaugeValueToAngle(value, g.max, g.isLog);

    if (active && !isLight) {
      ctx.save();
      ctx.globalAlpha = 0.4;
      ctx.beginPath();
      ctx.arc(cx, cy, R, GAUGE_ARC_START, va, false);
      ctx.strokeStyle = gColor;
      ctx.lineWidth = tw * 2.8;
      ctx.lineCap = 'round';
      ctx.filter = `blur(${Math.round(tw * 0.7)}px)`;
      ctx.stroke();
      ctx.restore();
    }

    ctx.save();
    ctx.globalAlpha = a;
    ctx.beginPath();
    ctx.arc(cx, cy, R, GAUGE_ARC_START, va, false);
    ctx.strokeStyle = gColor;
    ctx.lineWidth = tw;
    ctx.lineCap = 'round';
    if (active && !isLight) { ctx.shadowColor = gColor; ctx.shadowBlur = 10 * dpr; }
    ctx.stroke();
    ctx.restore();

    // tip dot
    ctx.save();
    ctx.globalAlpha = a;
    ctx.beginPath();
    ctx.arc(cx + R * Math.cos(va), cy + R * Math.sin(va), tw * 0.62, 0, 2 * Math.PI);
    ctx.fillStyle = cNum;
    if (!isLight) { ctx.shadowColor = gColor; ctx.shadowBlur = active ? 14 * dpr : 6 * dpr; }
    ctx.fill();
    ctx.restore();
  }

  // progress ring — stays visible while active even after progress hits 1.0,
  // because the test worker can report 100% a few seconds before testState changes
  if (active && progress > 0) {
    const ringEnd = GAUGE_ARC_START + Math.min(progress, 1) * GAUGE_ARC_SWEEP;
    ctx.save();
    ctx.globalAlpha = 0.45;
    ctx.beginPath();
    ctx.arc(cx, cy, R + tw * 1.05, GAUGE_ARC_START, ringEnd, false);
    ctx.strokeStyle = gColor;
    ctx.lineWidth = 2 * dpr;
    ctx.lineCap = 'round';
    ctx.stroke();
    ctx.restore();
  }

  // text
  const numY = cy + R * 0.28;
  const numSz = Math.round(R * 0.38);
  const uSz = Math.round(R * 0.13);
  const lSz = Math.round(R * 0.105);

  ctx.save();
  ctx.globalAlpha = dimmed ? 0.2 : (value > 0 ? 1 : (isLight ? 0.38 : 0.18));
  ctx.font = `200 ${numSz}px Inter,sans-serif`;
  ctx.fillStyle = cNum;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'alphabetic';
  ctx.fillText(value > 0 ? formatGaugeValue(value) : '–', cx, numY);
  ctx.restore();

  ctx.save();
  ctx.globalAlpha = dimmed ? 0.18 : (isLight ? 0.9 : 0.72);
  ctx.font = `500 ${uSz}px Inter,sans-serif`;
  ctx.fillStyle = gColor;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillText(g.unit, cx, numY + uSz * 0.3);
  ctx.restore();

  ctx.save();
  ctx.globalAlpha = dimmed ? 0.18 : (isLight ? 0.65 : 0.42);
  ctx.font = `700 ${lSz}px Inter,sans-serif`;
  ctx.fillStyle = cNum;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillText(g.label, cx, numY + uSz * 0.3 + uSz * 1.5);
  ctx.restore();
}

function formatGaugeValue(v) {
  v = Number(v);
  if (!v || isNaN(v)) return '–';
  if (v < 10) return v.toFixed(2);
  if (v < 100) return v.toFixed(1);
  return v.toFixed(0);
}
