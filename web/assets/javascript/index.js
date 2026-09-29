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

// ── Gauge config ────────────────────────────────────────────────────────────
// Gauge geometry, tick builders and the canvas draw routine live in the
// shared javascript/gauges.js, so this page and index.html stay in sync.
const GAUGES = {
  dl: {
    color: '#22d3ee',
    label: 'DOWNLOAD', unit: 'Mbps',
    isLog: true, max: 10000,
    ticks: buildGaugeLogTicks(),
    canvas: null,
  },
  ul: {
    color: '#a78bfa',
    label: 'UPLOAD', unit: 'Mbps',
    isLog: true, max: 10000,
    ticks: buildGaugeLogTicks(),
    canvas: null,
  },
  ping: {
    color: '#34d399',
    label: 'PING', unit: 'ms',
    isLog: false, max: 500,
    ticks: buildGaugeLinearTicks(
      [0, 50, 100, 150, 200, 250, 300, 400, 500],
      [0, 100, 200, 300, 500]),
    canvas: null,
  },
  jitter: {
    color: '#fbbf24',
    label: 'JITTER', unit: 'ms',
    isLog: false, max: 150,
    ticks: buildGaugeLinearTicks(
      [0, 25, 50, 75, 100, 125, 150],
      [0, 50, 100, 150]),
    canvas: null,
  },
};

// ── Number formatter ────────────────────────────────────────────────────────
// Unlike the shared formatGaugeValue() (which is only ever called for values
// already known to be > 0), this treats 0 as a real result — e.g. jitter=0
// on a LAN test is a valid measurement, not a missing one.
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
  Object.values(GAUGES).forEach(g => drawGauge(g.canvas, g, 0, 0, false, false));

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

      drawGauge(GAUGES.dl.canvas,     GAUGES.dl,     dlVal,     parseFloat(d.dlProgress)   || 0, ts === 1, running && ts !== 1 && !done);
      drawGauge(GAUGES.ul.canvas,     GAUGES.ul,     ulVal,     parseFloat(d.ulProgress)   || 0, ts === 3, running && ts !== 3 && !done);
      drawGauge(GAUGES.ping.canvas,   GAUGES.ping,   pingVal,   parseFloat(d.pingProgress) || 0, ts === 2, running && ts !== 2 && !done);
      drawGauge(GAUGES.jitter.canvas, GAUGES.jitter, jitterVal, 0,                               ts === 2, running && ts !== 2 && !done);

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
