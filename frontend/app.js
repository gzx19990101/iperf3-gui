'use strict';

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

const state = {
  view: 'client',
  engine: null,
  engineReady: false,
  protocol: 'tcp',
  direction: 'upload',
  running: false,
  runId: null,
  runStatus: null,
  lastSequence: 0,
  intervals: [],
  uploadSeries: [],
  downloadSeries: [],
  summary: null,
  historyId: '',
  config: null,
  startedAt: 0,
  progressTimer: null,
  previewTimer: null,
  historySearchTimer: null,
  server: {
    id: null,
    status: 'stopped',
    port: 5201,
    bind: '',
    oneOff: false,
    logs: [],
    connections: [],
    currentTest: null,
    intervals: [],
    uploadSeries: [],
    downloadSeries: [],
    summary: null,
    duration: 0,
    error: null,
    message: '',
    startedAt: '',
    endedAt: '',
  },
  history: {
    page: 1,
    pageSize: 50,
    total: 0,
    items: [],
    selected: new Set(),
    detail: null,
    detailPage: 1,
  },
};

function wailsApp() {
  return window.go && window.go.main && window.go.main.App;
}

async function call(method, ...args) {
  const app = wailsApp();
  if (!app || typeof app[method] !== 'function') {
    throw new Error('Wails 绑定尚未就绪');
  }
  return await app[method](...args);
}

function waitForRuntime() {
  return new Promise((resolve, reject) => {
    let attempts = 0;
    const check = () => {
      attempts += 1;
      if (wailsApp() && window.runtime) {
        resolve();
        return;
      }
      if (attempts >= 120) {
        reject(new Error('Wails 运行时未就绪'));
        return;
      }
      setTimeout(check, 50);
    };
    check();
  });
}

let toastTimer = null;
function showToast(message, kind = '') {
  const toast = $('#toast');
  toast.textContent = message;
  toast.className = 'toast is-visible' + (kind ? ` is-${kind}` : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    toast.className = 'toast';
  }, 3600);
}

const RATE_UNITS = [
  [1e12, 'Tbit/s'],
  [1e9, 'Gbit/s'],
  [1e6, 'Mbit/s'],
  [1e3, 'Kbit/s'],
  [1, 'bit/s'],
];

function formatRate(bps) {
  if (!Number.isFinite(bps) || bps <= 0) {
    return { value: '—', unit: '' };
  }
  for (const [scale, unit] of RATE_UNITS) {
    if (bps >= scale) {
      const value = bps / scale;
      return {
        value: value.toFixed(value >= 100 ? 0 : value >= 10 ? 1 : 2),
        unit,
      };
    }
  }
  return { value: String(Math.round(bps)), unit: 'bit/s' };
}

function rateText(bps) {
  const rate = formatRate(bps);
  return rate.unit ? `${rate.value} ${rate.unit}` : '—';
}

function compactRate(value) {
  const rate = formatRate(value);
  if (!rate.unit) return '0';
  return `${rate.value}${rate.unit.replace('bit/s', '')}`;
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(value >= 100 ? 0 : value >= 10 ? 1 : 2)} ${units[index]}`;
}

function formatSeconds(value) {
  if (!Number.isFinite(value) || value <= 0) return '—';
  return value >= 10 ? `${value.toFixed(1)} s` : `${value.toFixed(2)} s`;
}

function directionLabel(direction) {
  return direction === 'download' ? '下载' : direction === 'bidirectional' ? '双向' : '上传';
}

function protocolLabel(protocol) {
  return protocol === 'udp' ? 'UDP' : 'TCP';
}

function statusLabel(status) {
  const labels = {
    created: '已创建',
    starting: '启动中',
    running: '运行中',
    stopping: '停止中',
    stopped: '已停止',
    completed: '已完成',
    failed: '失败',
    interrupted: '已中断',
  };
  return labels[status] || '就绪';
}

function isActiveStatus(status) {
  return status === 'starting' || status === 'running' || status === 'stopping';
}

function statusPillClass(status) {
  if (isActiveStatus(status)) return 'state-pill is-running';
  if (status === 'failed' || status === 'interrupted') return 'state-pill is-error';
  return 'state-pill';
}

function setPill(element, status, override) {
  element.className = statusPillClass(status);
  element.innerHTML = '<span class="status-dot"></span><span></span>';
  const label = element.querySelector('span:last-child');
  label.textContent = override || statusLabel(status);
}

function localTime(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleTimeString('zh-CN', { hour12: false });
}

function localDateTime(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString('zh-CN', { hour12: false });
}

function intervalRange(item) {
  return `${Number(item.startSeconds || 0).toFixed(2)}–${Number(item.endSeconds || 0).toFixed(2)}s`;
}

function lossText(item) {
  const total = Number(item.udpTotalDatagrams || 0);
  if (!total) return '—';
  const lost = Number(item.udpLostDatagrams || 0);
  return `${lost}/${total} (${((lost / total) * 100).toFixed(2)}%)`;
}

function lossSummaryText(summary) {
  if (!summary || !summary.udpTotalDatagrams) return '—';
  const lost = Number(summary.udpLostDatagrams || 0);
  const total = Number(summary.udpTotalDatagrams || 0);
  const percent = summary.udpLostPercent ?? (total ? (lost / total) * 100 : 0);
  return `${lost}/${total} (${Number(percent).toFixed(2)}%)`;
}

function readNumber(id, fallback) {
  const value = Number($(`#${id}`).value);
  return Number.isFinite(value) ? value : fallback;
}

function bandwidthMultiplier(unit) {
  if (unit === 'G') return 1e9;
  if (unit === 'K') return 1e3;
  return 1e6;
}

function readClientConfig() {
  const bindInput = $('#bindAddress').value.trim();
  const interfaceValue = $('#interface').value;
  const config = {
    host: $('#host').value.trim(),
    port: readNumber('port', 5201),
    protocol: state.protocol,
    direction: state.direction,
    durationSeconds: readNumber('duration', 10),
    intervalSeconds: readNumber('interval', 1),
    parallelStreams: readNumber('parallel', 1),
    bindAddress: bindInput || interfaceValue || '',
    ipVersion: Number($('#ipVersion').value) || 0,
  };
  if (state.protocol === 'udp') {
    config.udpBandwidthBitsPerSecond = Math.round(
      readNumber('bandwidth', 100) * bandwidthMultiplier($('#bandwidthUnit').value)
    );
    config.udpDatagramLengthBytes = readNumber('udpLength', 1200);
  }
  return config;
}

function readServerConfig() {
  return {
    port: readNumber('serverPort', 5201),
    bindAddress: $('#serverBind').value || '',
    oneOff: state.server.oneOff,
  };
}

function directionSupported() {
  if (!state.engine) return true;
  if (state.protocol === 'udp') return Boolean(state.engine.capabilities && state.engine.capabilities.bidirUdp);
  return Boolean(state.engine.capabilities && state.engine.capabilities.bidirTcp);
}

function updateDirectionAvailability() {
  const button = $('#directionBidirectional');
  const supported = directionSupported();
  button.disabled = !supported;
  button.title = supported ? '' : '当前随包 iPerf3 不支持该协议的双向测试';
  if (!supported && state.direction === 'bidirectional') {
    state.direction = 'upload';
    $$('.segment-button[data-direction]').forEach((item) => {
      item.setAttribute('aria-pressed', String(item.dataset.direction === 'upload'));
    });
    updateDirectionHint();
  }
}

function updateDirectionHint() {
  const hints = {
    upload: '上传：客户端 → 服务端，测量客户端发送带宽。',
    download: '下载：服务端 → 客户端（-R），测量客户端接收带宽。',
    bidirectional: '双向：两端同时收发（--bidir），分别展示上传与下载。',
  };
  let hint = hints[state.direction] || '';
  if (state.protocol === 'udp' && state.direction === 'bidirectional') {
    hint += ' UDP 双向需要服务端预留多个 UDP 端口；本机 Cygwin 服务端存在已知限制，建议对支持该模式的服务端使用。';
  }
  $('#directionHint').textContent = hint;
}

function updateConfigKicker() {
  const arrows = {
    upload: '客户端 → 远端服务端',
    download: '远端服务端 → 客户端',
    bidirectional: '客户端 ⇄ 远端服务端',
  };
  $('#configKicker').textContent = `${directionLabel(state.direction)} · ${arrows[state.direction]}`;
}

function updateProtocolUI() {
  $$('.segment-button[data-protocol]').forEach((button) => {
    button.setAttribute('aria-pressed', String(button.dataset.protocol === state.protocol));
  });
  const isUdp = state.protocol === 'udp';
  $('#bandwidthField').hidden = !isUdp;
  $('#udpLengthField').hidden = !isUdp;
  $('#configProtocolBadge').textContent = state.protocol.toUpperCase();
  $('#configProtocolBadge').className = `protocol-badge${isUdp ? ' udp' : ''}`;
  updateDirectionAvailability();
  updateDirectionHint();
  updateConfigKicker();
  schedulePreview();
}

function setDirection(direction) {
  if (direction === 'bidirectional' && !directionSupported()) {
    showToast('当前协议不支持双向测试', 'error');
    return;
  }
  state.direction = direction;
  $$('.segment-button[data-direction]').forEach((button) => {
    button.setAttribute('aria-pressed', String(button.dataset.direction === direction));
  });
  updateDirectionHint();
  updateConfigKicker();
  schedulePreview();
}

function fallbackClientPreview(config) {
  const parts = [
    'iperf3',
    '-c',
    config.host || '<目标地址>',
    '-p',
    String(config.port || 5201),
    '-t',
    String(config.durationSeconds || 10),
    '-i',
    String(config.intervalSeconds || 1),
    '-P',
    String(config.parallelStreams || 1),
  ];
  if (config.protocol === 'udp') {
    parts.push('-u', '-b', String(config.udpBandwidthBitsPerSecond || 0), '-l', String(config.udpDatagramLengthBytes || 1200));
  }
  if (config.direction === 'download') parts.push('-R');
  if (config.direction === 'bidirectional') parts.push('--bidir');
  if (config.bindAddress) parts.push('-B', config.bindAddress);
  if (config.ipVersion === 4) parts.push('-4');
  if (config.ipVersion === 6) parts.push('-6');
  parts.push('--json-stream', '--forceflush');
  return parts.join(' ');
}

function fallbackServerPreview(config) {
  const parts = ['iperf3', '-s', '-p', String(config.port || 5201)];
  if (config.bindAddress) parts.push('-B', config.bindAddress);
  if (config.oneOff) parts.push('-1');
  return parts.join(' ');
}

function schedulePreview() {
  clearTimeout(state.previewTimer);
  state.previewTimer = setTimeout(() => {
    updateCommandPreview();
  }, 120);
}

async function updateCommandPreview() {
  const config = readClientConfig();
  try {
    const preview = await call('PreviewClientCommand', config);
    if (preview) $('#commandPreview').textContent = preview;
  } catch {
    $('#commandPreview').textContent = fallbackClientPreview(config);
  }
}

async function updateServerCommand() {
  const config = readServerConfig();
  try {
    const preview = await call('PreviewServerCommand', config);
    if (preview) $('#serverCommand').textContent = preview;
  } catch {
    $('#serverCommand').textContent = fallbackServerPreview(config);
  }
}

function setConfigLocked(locked) {
  ['host', 'port', 'interface', 'duration', 'interval', 'parallel', 'bandwidth', 'bandwidthUnit', 'udpLength', 'bindAddress', 'ipVersion'].forEach((id) => {
    const element = $(`#${id}`);
    if (element) element.disabled = locked;
  });
  $$('#view-client .segment-button').forEach((button) => {
    if (button.id === 'directionBidirectional') {
      button.disabled = locked || !directionSupported();
    } else {
      button.disabled = locked;
    }
  });
}

function setRunStatus(status, message = '') {
  state.runStatus = status;
  const active = isActiveStatus(status);
  state.running = active;
  setPill($('#resultState'), status);
  if (message) $('#resultTime').textContent = message;
  setConfigLocked(active);
  $('#startTest').disabled = active || !state.engineReady;
  $('#startTest').hidden = active;
  $('#stopTest').hidden = !active;
  $('#exportCurrent').disabled = active || !state.historyId;
  if (active) {
    $('#progressTrack').hidden = false;
  } else {
    stopProgress();
  }
}

function startProgress() {
  stopProgress();
  $('#progressTrack').hidden = false;
  state.progressTimer = setInterval(updateProgress, 250);
  updateProgress();
}

function stopProgress() {
  if (state.progressTimer) {
    clearInterval(state.progressTimer);
    state.progressTimer = null;
  }
  const fill = $('#progressFill');
  fill.classList.remove('is-indeterminate');
  fill.style.width = '0%';
  $('#progressTrack').hidden = true;
}

function updateProgress() {
  if (!state.config || !isActiveStatus(state.runStatus)) return;
  const fill = $('#progressFill');
  const duration = Number(state.config.durationSeconds || 0);
  if (duration <= 0) {
    fill.classList.add('is-indeterminate');
    fill.style.width = '35%';
    return;
  }
  fill.classList.remove('is-indeterminate');
  const elapsed = Math.max(0, (Date.now() - state.startedAt) / 1000);
  const percent = Math.min(100, (elapsed / duration) * 100);
  fill.style.width = `${percent}%`;
}

function resetRunView() {
  state.intervals = [];
  state.uploadSeries = [];
  state.downloadSeries = [];
  state.summary = null;
  state.historyId = '';
  state.lastSequence = 0;
  state.runStatus = null;
  $('#runErrorBox').hidden = true;
  $('#runErrorBox').textContent = '';
  $('#rawOutput').textContent = '';
  $('#rawOutputSummary').textContent = 'iPerf3 原始输出';
  $('#exportCurrent').disabled = true;
  $('#resultTarget').textContent = '正在启动测试…';
  $('#resultTime').textContent = '';
  $('#progressFill').style.width = '0%';
  renderMetrics();
  renderChart();
  renderIntervalTable();
}

async function startTest() {
  if (!state.engineReady || state.running) return;
  const config = readClientConfig();
  $('#formError').textContent = '';
  resetRunView();
  try {
    const result = await call('StartClientTest', config);
    if (result.error) {
      $('#formError').textContent = result.error.message || '无法启动测试';
      return;
    }
    state.runId = result.runId;
    state.config = config;
    state.startedAt = Date.now();
    state.lastSequence = 0;
    $('#resultTarget').textContent = `${config.host}:${config.port} · ${protocolLabel(config.protocol)} · ${directionLabel(config.direction)}`;
    $('#resultTime').textContent = '正在启动测试';
    setRunStatus('starting', '正在启动测试');
    startProgress();
    const snapshot = await call('GetRunSnapshot', result.runId);
    applySnapshot(snapshot);
  } catch (error) {
    $('#formError').textContent = error.message || String(error);
  }
}

async function stopTest() {
  if (!state.runId) return;
  try {
    const result = await call('StopClientTest', state.runId);
    if (result.error) {
      showToast(result.error.message || '停止失败', 'error');
      return;
    }
    setRunStatus('stopping', '正在停止测试');
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

function mergeIntervals(intervals) {
  if (!Array.isArray(intervals) || intervals.length === 0) return;
  const seen = new Set(state.intervals.map((item) => `${item.sequence}|${item.direction}`));
  let added = false;
  for (const item of intervals) {
    const key = `${item.sequence}|${item.direction}`;
    if (seen.has(key)) continue;
    seen.add(key);
    state.intervals.push(item);
    const series = item.direction === 'download' ? state.downloadSeries : state.uploadSeries;
    series.push(Number(item.bitsPerSecond || 0));
    if (series.length > 300) series.splice(0, series.length - 300);
    added = true;
  }
  if (!added) return;
  state.intervals.sort((left, right) => {
    if (left.sequence !== right.sequence) return left.sequence - right.sequence;
    return left.direction === 'upload' ? -1 : 1;
  });
  if (state.intervals.length > 2000) {
    state.intervals.splice(0, state.intervals.length - 2000);
  }
  renderMetrics();
  renderChart();
  renderIntervalTable();
}

function currentSeriesValue(direction, summaryKey) {
  if (state.summary && state.summary[summaryKey]) {
    return state.summary[summaryKey].bitsPerSecond;
  }
  for (let index = state.intervals.length - 1; index >= 0; index -= 1) {
    if (state.intervals[index].direction === direction) {
      return state.intervals[index].bitsPerSecond;
    }
  }
  return null;
}

function renderMetricCard(container, label, value, unit, foot, accent) {
  const card = document.createElement('div');
  card.className = 'metric';
  const labelElement = document.createElement('div');
  labelElement.className = 'metric-label';
  labelElement.textContent = label;
  const valueElement = document.createElement('div');
  valueElement.className = `metric-value${accent ? ` metric-accent-${accent}` : ''}`;
  const strong = document.createElement('strong');
  strong.textContent = value;
  valueElement.append(strong);
  if (unit) {
    const unitElement = document.createElement('span');
    unitElement.textContent = unit;
    valueElement.append(unitElement);
  }
  const footElement = document.createElement('div');
  footElement.className = 'metric-foot';
  footElement.textContent = foot;
  card.append(labelElement, valueElement, footElement);
  container.append(card);
}

function renderMetricsInto(container, protocol, direction, summary, intervals, duration, sourceLabel = '接收端测量') {
  container.replaceChildren();
  const uploadBps = summary && summary.upload ? summary.upload.bitsPerSecond : findLastBps(intervals, 'upload');
  const downloadBps = summary && summary.download ? summary.download.bitsPerSecond : findLastBps(intervals, 'download');
  const uploadRate = formatRate(uploadBps);
  const downloadRate = formatRate(downloadBps);
  renderMetricCard(
    container,
    '上传吞吐',
    direction === 'download' ? '—' : uploadRate.value,
    direction === 'download' ? '' : uploadRate.unit,
    direction === 'download' ? '未启用上传' : '客户端 → 服务端',
    'blue'
  );
  renderMetricCard(
    container,
    '下载吞吐',
    direction === 'upload' ? '—' : downloadRate.value,
    direction === 'upload' ? '' : downloadRate.unit,
    direction === 'upload' ? '未启用下载' : '服务端 → 客户端',
    'orange'
  );

  if (protocol === 'udp') {
    const jitter = summary
      ? Math.max(
          summary.upload ? Number(summary.upload.udpJitterMilliseconds || 0) : 0,
          summary.download ? Number(summary.download.udpJitterMilliseconds || 0) : 0
        )
      : findLastNumber(intervals, 'udpJitterMilliseconds');
    const lost = summary
      ? (summary.upload ? Number(summary.upload.udpLostDatagrams || 0) : 0) +
        (summary.download ? Number(summary.download.udpLostDatagrams || 0) : 0)
      : sumNumbers(intervals, 'udpLostDatagrams');
    const total = summary
      ? (summary.upload ? Number(summary.upload.udpTotalDatagrams || 0) : 0) +
        (summary.download ? Number(summary.download.udpTotalDatagrams || 0) : 0)
      : sumNumbers(intervals, 'udpTotalDatagrams');
    renderMetricCard(container, '抖动', jitter > 0 ? jitter.toFixed(2) : '—', jitter > 0 ? 'ms' : '', sourceLabel, 'green');
    renderMetricCard(
      container,
      '丢包',
      total > 0 ? String(lost) : '—',
      total > 0 ? `/ ${total}` : '',
      total > 0 ? `${((lost / total) * 100).toFixed(2)}% 丢包` : sourceLabel,
      total > 0 && lost > 0 ? 'orange' : 'green'
    );
  } else {
    const retransmits = summary
      ? (summary.upload ? Number(summary.upload.tcpRetransmits || 0) : 0) +
        (summary.download ? Number(summary.download.tcpRetransmits || 0) : 0)
      : sumNumbers(intervals, 'tcpRetransmits');
    const seconds = summary
      ? Math.max(
          summary.upload ? Number(summary.upload.seconds || 0) : 0,
          summary.download ? Number(summary.download.seconds || 0) : 0
        )
      : duration;
    renderMetricCard(container, 'TCP 重传', String(retransmits), '次', '上传 + 下载合计', retransmits > 0 ? 'orange' : 'green');
    const indefinite = !seconds && duration === 0;
    renderMetricCard(
      container,
      '时长',
      indefinite ? '不限时' : seconds ? seconds.toFixed(1) : '—',
      indefinite ? '' : seconds ? 's' : '',
      indefinite ? '手动停止' : '本次测试累计',
      'green'
    );
  }
}

function findLastBps(intervals, direction) {
  for (let index = intervals.length - 1; index >= 0; index -= 1) {
    if (intervals[index].direction === direction) return intervals[index].bitsPerSecond;
  }
  return null;
}

function findLastNumber(intervals, key) {
  for (let index = intervals.length - 1; index >= 0; index -= 1) {
    const value = Number(intervals[index][key] || 0);
    if (value > 0) return value;
  }
  return 0;
}

function sumNumbers(intervals, key) {
  return intervals.reduce((total, item) => total + Number(item[key] || 0), 0);
}

function renderMetrics() {
  const protocol = state.config ? state.config.protocol : state.protocol;
  const direction = state.config ? state.config.direction : state.direction;
  const duration = state.config ? state.config.durationSeconds : null;
  renderMetricsInto($('#metrics'), protocol, direction, state.summary, state.intervals, duration);
}

function svgElement(tag, attributes) {
  const element = document.createElementNS('http://www.w3.org/2000/svg', tag);
  Object.entries(attributes).forEach(([key, value]) => element.setAttribute(key, value));
  return element;
}

function chartPath(values, maxValue) {
  if (!values.length) return '';
  const count = Math.max(values.length - 1, 1);
  return values
    .map((value, index) => {
      const x = 70 + (800 * index) / count;
      const y = 202 - (Math.min(value, maxValue) / maxValue) * 182;
      return `${index === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
}

function clientChartElements() {
  return {
    grid: $('#chartGrid'),
    yLabels: $('#chartYAxisLabels'),
    xLabels: $('#chartLabels'),
    points: $('#chartPoints'),
    uploadPath: $('#uploadPath'),
    downloadPath: $('#downloadPath'),
    area: $('#chartArea'),
    empty: $('#chartEmpty'),
    legend: $('#chartLegend'),
  };
}

function serverChartElements() {
  return {
    grid: $('#serverChartGrid'),
    yLabels: $('#serverChartYAxisLabels'),
    xLabels: $('#serverChartLabels'),
    points: $('#serverChartPoints'),
    uploadPath: $('#serverUploadPath'),
    downloadPath: $('#serverDownloadPath'),
    area: $('#serverChartArea'),
    empty: $('#serverChartEmpty'),
    legend: $('#serverChartLegend'),
  };
}

function renderChart() {
  const target = state.config && state.config.protocol === 'udp' ? state.config.udpBandwidthBitsPerSecond : 0;
  renderThroughputChart(clientChartElements(), state.uploadSeries, state.downloadSeries, state.intervals, target, 'url(#uploadFill)', 'url(#downloadFill)');
}

function renderServerChart() {
  renderThroughputChart(serverChartElements(), state.server.uploadSeries, state.server.downloadSeries, state.server.intervals, 0, 'url(#serverUploadFill)', 'url(#serverDownloadFill)');
}

function renderThroughputChart(elements, upload, download, intervals, maxTarget, uploadFill, downloadFill) {
  elements.grid.replaceChildren();
  elements.yLabels.replaceChildren();
  elements.xLabels.replaceChildren();
  elements.points.replaceChildren();
  elements.uploadPath.setAttribute('d', '');
  elements.downloadPath.setAttribute('d', '');
  elements.area.setAttribute('d', '');

  const all = upload.concat(download);
  if (!all.length) {
    elements.empty.hidden = false;
    renderLegendInto(elements.legend, false, false);
    return;
  }
  elements.empty.hidden = true;

  let maxValue = Math.max(...all, 1) * 1.1;
  if (Number(maxTarget) > 0) {
    maxValue = Math.max(maxValue, Number(maxTarget) * 1.1);
  }
  for (let index = 0; index < 5; index += 1) {
    const value = (maxValue * index) / 4;
    const y = 202 - (index * 182) / 4;
    elements.grid.append(svgElement('line', { x1: 70, x2: 870, y1: y, y2: y, class: 'chart-gridline' }));
    const text = svgElement('text', { x: 60, y: y + 3, 'text-anchor': 'end', class: 'chart-label' });
    text.textContent = compactRate(value);
    elements.yLabels.append(text);
  }

  const uploadD = chartPath(upload, maxValue);
  const downloadD = chartPath(download, maxValue);
  elements.uploadPath.setAttribute('d', uploadD);
  elements.downloadPath.setAttribute('d', downloadD);
  if (uploadD) {
    elements.area.setAttribute('d', `${uploadD} L870,202 L70,202 Z`);
    elements.area.setAttribute('fill', uploadFill);
  } else if (downloadD) {
    elements.area.setAttribute('d', `${downloadD} L870,202 L70,202 Z`);
    elements.area.setAttribute('fill', downloadFill);
  }

  const drawPoint = (values, color) => {
    if (!values.length) return;
    const count = Math.max(values.length - 1, 1);
    const x = 70 + (800 * (values.length - 1)) / count;
    const y = 202 - (Math.min(values[values.length - 1], maxValue) / maxValue) * 182;
    elements.points.append(svgElement('circle', { cx: x, cy: y, r: 3.5, class: 'chart-point', style: `stroke:${color}` }));
  };
  drawPoint(upload, '#1769e0');
  drawPoint(download, '#dc7948');

  const first = intervals[0];
  const last = intervals[intervals.length - 1];
  if (first && last) {
    const start = Number(first.startSeconds || 0);
    const end = Number(last.endSeconds || 0);
    [0, 1 / 3, 2 / 3, 1].forEach((fraction, index) => {
      const text = svgElement('text', { x: 70 + (800 * fraction), y: 232, 'text-anchor': index === 0 ? 'start' : index === 3 ? 'end' : 'middle', class: 'chart-label' });
      text.textContent = `${(start + (end - start) * fraction).toFixed(1)}s`;
      elements.xLabels.append(text);
    });
  }
  renderLegendInto(elements.legend, upload.length > 0, download.length > 0);
}

function renderLegendInto(legend, hasUpload, hasDownload) {
  legend.replaceChildren();
  const add = (label, className) => {
    const item = document.createElement('span');
    item.className = 'legend-item';
    const line = document.createElement('span');
    line.className = `legend-line ${className}`;
    const text = document.createElement('span');
    text.textContent = label;
    item.append(line, text);
    legend.append(item);
  };
  if (hasUpload) add('上传', '');
  if (hasDownload) add('下载', 'sender');
}

function intervalColumns(protocol, direction) {
  if (direction === 'bidirectional') {
    const columns = [
      { label: '区间' },
      { label: '方向' },
      { label: '传输量', numeric: true },
      { label: '带宽', numeric: true },
      { label: protocol === 'udp' ? '抖动' : '重传', numeric: true },
    ];
    if (protocol === 'udp') columns.push({ label: '丢包', numeric: true });
    return columns;
  }
  const columns = [
    { label: '区间' },
    { label: '传输量', numeric: true },
    { label: `${directionLabel(direction)}带宽`, numeric: true },
    { label: protocol === 'udp' ? '抖动' : '重传', numeric: true },
  ];
  if (protocol === 'udp') columns.push({ label: '丢包', numeric: true });
  return columns;
}

function groupIntervals(intervals) {
  const groups = new Map();
  for (const item of intervals) {
    if (!groups.has(item.sequence)) groups.set(item.sequence, []);
    groups.get(item.sequence).push(item);
  }
  return Array.from(groups.entries())
    .sort((left, right) => left[0] - right[0])
    .map(([, items]) => items.sort((left, right) => (left.direction === 'upload' ? -1 : 1)));
}

function buildIntervalRow(item, protocol, direction, showDirection) {
  const row = document.createElement('tr');
  const values = [];
  values.push({ text: intervalRange(item) });
  if (showDirection) values.push({ text: directionLabel(item.direction) });
  values.push({ text: formatBytes(item.bytes), numeric: true });
  values.push({ text: rateText(item.bitsPerSecond), numeric: true });
  if (protocol === 'udp') {
    values.push({ text: item.udpJitterMilliseconds ? `${Number(item.udpJitterMilliseconds).toFixed(2)} ms` : '—', numeric: true });
    values.push({ text: lossText(item), numeric: true });
  } else {
    values.push({ text: String(item.tcpRetransmits || 0), numeric: true });
  }
  for (const value of values) {
    const cell = document.createElement('td');
    cell.textContent = value.text;
    if (value.numeric) cell.className = 'numeric';
    row.append(cell);
  }
  return row;
}

function renderIntervalTableInto(head, rows, intervals, protocol, direction, noteElement, noteText) {
  head.replaceChildren();
  rows.replaceChildren();
  const columns = intervalColumns(protocol, direction);
  for (const column of columns) {
    const cell = document.createElement('th');
    cell.textContent = column.label;
    if (column.numeric) cell.className = 'numeric';
    head.append(cell);
  }
  const groups = groupIntervals(intervals);
  if (!groups.length) {
    const row = document.createElement('tr');
    row.className = 'empty-row';
    const cell = document.createElement('td');
    cell.colSpan = columns.length;
    cell.textContent = '等待测试数据';
    row.append(cell);
    rows.append(row);
  } else {
    const visible = groups.slice(-200);
    for (const group of visible) {
      if (direction === 'bidirectional') {
        for (const item of group) rows.append(buildIntervalRow(item, protocol, direction, true));
      } else {
        rows.append(buildIntervalRow(group[0], protocol, direction, false));
      }
    }
  }
  if (noteElement) {
    noteElement.textContent = noteText || `${groups.length} 个区间 · ${protocolLabel(protocol)} · ${directionLabel(direction)}`;
  }
}

function renderIntervalTable() {
  const protocol = state.config ? state.config.protocol : state.protocol;
  const direction = state.config ? state.config.direction : state.direction;
  renderIntervalTableInto($('#intervalHead'), $('#intervalRows'), state.intervals, protocol, direction, $('#intervalNote'));
}

function setRawOutput(rawOutput, rawTruncated, stderrSummary) {
  let text = rawOutput || '';
  if (stderrSummary) {
    text += `${text ? '\n\n' : ''}--- stderr ---\n${stderrSummary}`;
  }
  $('#rawOutput').textContent = text;
  const lines = text ? text.split('\n').length : 0;
  $('#rawOutputSummary').textContent = `iPerf3 原始输出${rawTruncated ? '（已截断）' : ''}${lines ? ` · ${lines} 行` : ''}`;
}

function showRunError(error, stderrSummary) {
  const box = $('#runErrorBox');
  if (!error) {
    box.hidden = true;
    box.textContent = '';
    return;
  }
  const lines = [];
  if (error.code) lines.push(`[${error.code}] ${error.message || ''}`);
  else lines.push(error.message || '测试失败');
  if (error.detail) lines.push(error.detail);
  if (stderrSummary) lines.push(`stderr: ${stderrSummary}`);
  box.textContent = lines.join('\n');
  box.hidden = false;
}

function applySnapshot(snapshot) {
  if (!snapshot || snapshot.runId !== state.runId) return;
  if (snapshot.error) {
    showRunError(snapshot.error, snapshot.stderrSummary);
  }
  state.lastSequence = Math.max(state.lastSequence, Number(snapshot.lastSequence || 0));
  if (snapshot.summary) state.summary = snapshot.summary;
  if (snapshot.historyId) state.historyId = snapshot.historyId;
  if (Array.isArray(snapshot.latestIntervals)) mergeIntervals(snapshot.latestIntervals);
  if (snapshot.status) {
    setRunStatus(snapshot.status, snapshot.message || '');
  }
  if (snapshot.rawOutput || snapshot.stderrSummary) {
    setRawOutput(snapshot.rawOutput, snapshot.rawTruncated, snapshot.stderrSummary);
  }
  if (snapshot.error) showRunError(snapshot.error, snapshot.stderrSummary);
  if (snapshot.summary) state.summary = snapshot.summary;
  renderMetrics();
  renderChart();
  renderIntervalTable();
  $('#exportCurrent').disabled = isActiveStatus(snapshot.status) || !state.historyId;
}

async function refreshSnapshot() {
  if (!state.runId) return;
  try {
    const snapshot = await call('GetRunSnapshot', state.runId);
    applySnapshot(snapshot);
  } catch {
    return;
  }
}

function subscribeEvents() {
  if (!window.runtime || typeof window.runtime.EventsOn !== 'function') return;
  window.runtime.EventsOn('run:state', (payload) => {
    if (!payload || payload.runId !== state.runId) return;
    if (payload.sequence && payload.sequence <= state.lastSequence) return;
    state.lastSequence = Math.max(state.lastSequence, Number(payload.sequence || 0));
    setRunStatus(payload.status, payload.message || '');
    if (payload.status === 'running') {
      state.startedAt = Date.now();
      startProgress();
    }
    if (payload.status === 'stopped') {
      stopProgress();
      refreshSnapshot();
    }
  });
  window.runtime.EventsOn('run:interval', (payload) => {
    if (!payload || payload.runId !== state.runId) return;
    if (payload.sequence && payload.sequence <= state.lastSequence) return;
    state.lastSequence = Math.max(state.lastSequence, Number(payload.sequence || 0));
    mergeIntervals(payload.intervals || []);
  });
  window.runtime.EventsOn('run:completed', async (payload) => {
    if (!payload || payload.runId !== state.runId) return;
    state.lastSequence = Math.max(state.lastSequence, Number(payload.sequence || 0));
    if (payload.summary) state.summary = payload.summary;
    if (payload.historyId) state.historyId = payload.historyId;
    setRunStatus('completed', '测试完成');
    stopProgress();
    $('#progressFill').style.width = '100%';
    $('#exportCurrent').disabled = !state.historyId;
    await refreshSnapshot();
    renderMetrics();
    renderChart();
    renderIntervalTable();
    if (state.view === 'history') loadHistory(state.history.page);
  });
  window.runtime.EventsOn('run:failed', async (payload) => {
    if (!payload || payload.runId !== state.runId) return;
    state.lastSequence = Math.max(state.lastSequence, Number(payload.sequence || 0));
    setRunStatus('failed', '测试失败');
    stopProgress();
    showRunError(payload.error, payload.stderrSummary);
    await refreshSnapshot();
    if (state.view === 'history') loadHistory(state.history.page);
  });
  window.runtime.EventsOn('server:state', (payload) => {
    if (!payload) return;
    applyServerSnapshot(payload);
  });
  window.runtime.EventsOn('server:log', (payload) => {
    if (!payload) return;
    if (state.server.id && payload.serverId !== state.server.id) return;
    state.server.id = payload.serverId;
    state.server.logs.push({ timestamp: payload.timestamp, line: payload.line });
    if (state.server.logs.length > 500) state.server.logs.splice(0, state.server.logs.length - 500);
    renderServerLogs();
  });
}

function applyEngineStatus(status) {
  state.engine = status;
  state.engineReady = Boolean(status && status.available);
  const dot = $('#engineDot');
  dot.style.background = '';
  if (state.engineReady) {
    dot.className = 'status-dot';
    $('#engineText').textContent = '引擎就绪';
    $('#engineVersion').textContent = `iPerf3 ${status.version || ''}`;
    $('#formError').textContent = '';
    $('#serverError').textContent = '';
    $('#startTest').disabled = isActiveStatus(state.runStatus);
    $('#startServer').disabled = isActiveStatus(state.server.status);
  } else {
    dot.className = 'status-dot';
    dot.style.background = 'var(--red)';
    $('#engineText').textContent = '引擎不可用';
    $('#engineVersion').textContent = (status && status.error && status.error.code) || '';
    const message = (status && status.error && status.error.message) || '随包 iPerf3 不可用，请修复安装';
    $('#formError').textContent = message;
    $('#serverError').textContent = message;
    $('#startTest').disabled = true;
    $('#startServer').disabled = true;
  }
  populateInterfaces((status && status.interfaces) || []);
  updateDirectionAvailability();
  schedulePreview();
  updateServerCommand();
  renderServer();
}

function populateInterfaces(interfaces) {
  const clientSelect = $('#interface');
  const serverSelect = $('#serverBind');
  const previousClient = clientSelect.value;
  const previousServer = serverSelect.value;
  clientSelect.replaceChildren(new Option('自动选择', ''));
  serverSelect.replaceChildren(new Option('所有接口', ''));
  for (const iface of interfaces) {
    if (!iface || !iface.address) continue;
    const label = `${iface.name || '接口'} · ${iface.address}`;
    clientSelect.add(new Option(label, iface.address));
    serverSelect.add(new Option(label, iface.address));
  }
  if ([...clientSelect.options].some((option) => option.value === previousClient)) {
    clientSelect.value = previousClient;
  }
  if ([...serverSelect.options].some((option) => option.value === previousServer)) {
    serverSelect.value = previousServer;
  }
}

function applyServerSnapshot(snapshot) {
  if (state.server.id && snapshot.serverId !== state.server.id) return;
  state.server.id = snapshot.serverId;
  state.server.status = snapshot.status || 'stopped';
  state.server.port = snapshot.port || state.server.port;
  state.server.bind = snapshot.bindAddress || '';
  state.server.connections = snapshot.connections || [];
  state.server.logs = (snapshot.logs || []).map((line) => ({ timestamp: '', line }));
  state.server.error = snapshot.error || null;
  state.server.message = snapshot.message || '';
  state.server.startedAt = snapshot.startedAt || '';
  state.server.endedAt = snapshot.endedAt || '';
  state.server.currentTest = snapshot.currentTest || null;
  state.server.intervals = [];
  state.server.uploadSeries = [];
  state.server.downloadSeries = [];
  state.server.summary = snapshot.currentTest ? snapshot.currentTest.summary || null : null;
  state.server.duration = snapshot.currentTest ? Number(snapshot.currentTest.durationSeconds || 0) : 0;
  for (const item of (snapshot.currentTest && snapshot.currentTest.intervals) || []) {
    state.server.intervals.push(item);
    const series = item.direction === 'download' ? state.server.downloadSeries : state.server.uploadSeries;
    series.push(Number(item.bitsPerSecond || 0));
  }
  renderServer();
}

function renderServer() {
  const status = state.server.status;
  setPill($('#serverStatePill'), status);
  const titles = {
    starting: '启动中',
    running: '正在监听',
    stopping: '停止中',
    stopped: '已停止',
    failed: '启动失败',
  };
  $('#serverStateTitle').textContent = titles[status] || '未监听';
  $('#serverStateDetail').textContent = `端口 ${state.server.port} · ${state.server.bind || '所有接口'}`;
  $('#serverStateTime').textContent = state.server.endedAt
    ? `结束于 ${localTime(state.server.endedAt)}`
    : state.server.startedAt
      ? `启动于 ${localTime(state.server.startedAt)}`
      : '';
  const active = isActiveStatus(status);
  $('#startServer').hidden = active;
  $('#startServer').disabled = active || !state.engineReady;
  $('#stopServer').hidden = !active;
  const errorBox = $('#serverRunError');
  if (state.server.error) {
    const lines = [`[${state.server.error.code}] ${state.server.error.message}`];
    if (state.server.error.detail) lines.push(state.server.error.detail);
    errorBox.textContent = lines.join('\n');
    errorBox.hidden = false;
  } else {
    errorBox.hidden = true;
    errorBox.textContent = '';
  }
  renderServerTest();
  renderServerConnections();
  renderServerLogs();
}

function renderServerTest() {
  const test = state.server.currentTest;
  if (!test) {
    $('#serverTestTarget').textContent = '等待客户端连接';
    const pill = $('#serverTestState');
    pill.className = 'state-pill';
    pill.innerHTML = '<span class="status-dot"></span><span>—</span>';
    $('#serverMetrics').replaceChildren();
    renderServerChart();
    renderIntervalTableInto($('#serverIntervalHead'), $('#serverIntervalRows'), [], 'tcp', 'upload', $('#serverIntervalNote'), '等待客户端连接');
    return;
  }
  $('#serverTestTarget').textContent = `${test.client || '—'} · ${protocolLabel(test.protocol)} · ${directionLabel(test.direction)}`;
  setPill($('#serverTestState'), test.status);
  renderMetricsInto($('#serverMetrics'), test.protocol, test.direction, test.summary, test.intervals || [], test.durationSeconds || 0, '服务端测量');
  renderServerChart();
  renderIntervalTableInto(
    $('#serverIntervalHead'),
    $('#serverIntervalRows'),
    test.intervals || [],
    test.protocol,
    test.direction,
    $('#serverIntervalNote'),
    `${(test.intervals || []).length} 个区间 · ${protocolLabel(test.protocol)} · ${directionLabel(test.direction)}`
  );
}

function connectionStatusPill(status) {
  const text = String(status || '');
  const pill = document.createElement('span');
  let className = 'state-pill';
  if (text === '运行中' || text === '已连接') className += ' is-running';
  if (text === '失败' || text === '已中断') className += ' is-error';
  pill.className = className;
  pill.innerHTML = '<span class="status-dot"></span><span></span>';
  pill.querySelector('span:last-child').textContent = text || '—';
  return pill;
}

function renderServerConnections() {
  const rows = $('#serverConnectionRows');
  rows.replaceChildren();
  const connections = state.server.connections || [];
  $('#connectionCount').textContent = `${connections.length} 个连接`;
  if (!connections.length) {
    const row = document.createElement('tr');
    row.className = 'empty-row';
    const cell = document.createElement('td');
    cell.colSpan = 8;
    cell.textContent = '暂无客户端连接';
    row.append(cell);
    rows.append(row);
    return;
  }
  for (const connection of connections) {
    const row = document.createElement('tr');
    const values = [
      { text: connection.client || '—' },
      { text: String(connection.protocol || '').toUpperCase() || '—' },
      { text: connection.direction ? directionLabel(connection.direction) : '—' },
      { text: rateText(connection.uploadBitsPerSecond), numeric: true },
      { text: rateText(connection.downloadBitsPerSecond), numeric: true },
      { text: connection.startedAt || '—' },
      { text: connection.endedAt || '—' },
    ];
    for (const value of values) {
      const cell = document.createElement('td');
      cell.textContent = value.text;
      if (value.numeric) cell.className = 'numeric';
      row.append(cell);
    }
    const statusCell = document.createElement('td');
    statusCell.className = 'numeric';
    statusCell.append(connectionStatusPill(connection.status));
    row.append(statusCell);
    rows.append(row);
  }
}

function renderServerLogs() {
  const list = $('#serverLogs');
  list.replaceChildren();
  const logs = (state.server.logs || []).slice(-200);
  $('#serverLogCount').textContent = logs.length ? `${logs.length} 行` : '实时';
  if (!logs.length) {
    const item = document.createElement('li');
    const time = document.createElement('time');
    time.textContent = '--:--:--';
    const text = document.createElement('span');
    text.textContent = '服务端未启动';
    item.append(time, text);
    list.append(item);
    return;
  }
  for (const log of logs) {
    const item = document.createElement('li');
    const time = document.createElement('time');
    time.textContent = log.timestamp || '--:--:--';
    const text = document.createElement('span');
    text.textContent = log.line;
    item.append(time, text);
    list.append(item);
  }
}

async function startServer() {
  const config = readServerConfig();
  $('#serverError').textContent = '';
  $('#serverRunError').hidden = true;
  try {
    const result = await call('StartServer', config);
    if (result.error) {
      $('#serverError').textContent = result.error.message || '无法启动服务端';
      return;
    }
    state.server.id = result.serverId;
    state.server.status = 'starting';
    state.server.port = config.port;
    state.server.bind = config.bindAddress;
    state.server.oneOff = config.oneOff;
    state.server.logs = [];
    state.server.connections = [];
    state.server.error = null;
    renderServer();
  } catch (error) {
    $('#serverError').textContent = error.message || String(error);
  }
}

async function stopServer() {
  if (!state.server.id) return;
  try {
    const result = await call('StopServer', state.server.id);
    if (result.error) {
      showToast(result.error.message || '停止服务端失败', 'error');
      return;
    }
    state.server.status = 'stopping';
    renderServer();
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

function setOneOff(value) {
  state.server.oneOff = Boolean(value);
  $('#oneOffSwitch').setAttribute('aria-checked', String(state.server.oneOff));
  updateServerCommand();
}

async function loadHistory(page = 1) {
  const filter = {
    query: $('#historySearch').value.trim(),
    protocol: $('#historyProtocol').value,
    direction: $('#historyDirection').value,
    page,
    pageSize: state.history.pageSize,
  };
  try {
    const result = await call('ListHistory', filter);
    if (result.error) {
      showToast(result.error.message || '查询历史失败', 'error');
      return;
    }
    state.history.items = result.items || [];
    state.history.total = Number(result.total || 0);
    state.history.page = Number(result.page || page);
    state.history.selected.clear();
    renderHistory();
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

function renderHistory() {
  const rows = $('#historyRows');
  rows.replaceChildren();
  const items = state.history.items || [];
  $('#historyCount').textContent = `${state.history.total} 条`;
  if (!items.length) {
    const row = document.createElement('tr');
    row.className = 'empty-row';
    const cell = document.createElement('td');
    cell.colSpan = 9;
    cell.textContent = '暂无历史记录';
    row.append(cell);
    rows.append(row);
  } else {
    for (const item of items) {
      const row = document.createElement('tr');
      if (state.history.selected.has(item.id)) row.classList.add('is-selected');
      const checkCell = document.createElement('td');
      const checkbox = document.createElement('input');
      checkbox.type = 'checkbox';
      checkbox.checked = state.history.selected.has(item.id);
      checkbox.setAttribute('aria-label', `选择 ${item.host}:${item.port}`);
      checkbox.addEventListener('click', (event) => event.stopPropagation());
      checkbox.addEventListener('change', () => {
        if (checkbox.checked) state.history.selected.add(item.id);
        else state.history.selected.delete(item.id);
        row.classList.toggle('is-selected', checkbox.checked);
        updateHistoryButtons();
        updateSelectAll();
      });
      checkCell.append(checkbox);
      row.append(checkCell);
      const values = [
        localDateTime(item.createdAt),
        `${item.host}:${item.port}`,
        protocolLabel(item.protocol),
        directionLabel(item.direction),
        rateText(item.uploadBitsPerSecond),
        rateText(item.downloadBitsPerSecond),
        item.durationSeconds > 0 ? `${item.durationSeconds} s` : '不限时',
      ];
      values.forEach((value, index) => {
        const cell = document.createElement('td');
        cell.textContent = value;
        if (index === 4 || index === 5 || index === 6) cell.className = 'numeric';
        row.append(cell);
      });
      const statusCell = document.createElement('td');
      const pill = document.createElement('span');
      setPill(pill, item.status);
      statusCell.append(pill);
      row.append(statusCell);
      row.addEventListener('click', () => loadHistoryDetail(item.id));
      rows.append(row);
    }
  }
  const totalPages = Math.max(1, Math.ceil(state.history.total / state.history.pageSize));
  $('#historyPageLabel').textContent = `第 ${state.history.page} 页 / 共 ${totalPages} 页`;
  $('#prevHistory').disabled = state.history.page <= 1;
  $('#nextHistory').disabled = state.history.page >= totalPages;
  updateHistoryButtons();
  updateSelectAll();
}

function updateHistoryButtons() {
  const selected = state.history.selected.size;
  $('#deleteHistory').disabled = selected === 0;
  $('#exportHistory').disabled = selected !== 1 && !state.history.detail;
}

function updateSelectAll() {
  const items = state.history.items || [];
  const selected = state.history.selected.size;
  $('#selectAllHistory').checked = items.length > 0 && selected === items.length;
  $('#selectAllHistory').indeterminate = selected > 0 && selected < items.length;
}

async function loadHistoryDetail(id, page = 1) {
  try {
    const result = await call('GetHistoryResult', id, page, 200);
    if (result.error) {
      showToast(result.error.message || '读取历史详情失败', 'error');
      return;
    }
    state.history.detail = result;
    state.history.detailPage = page;
    renderHistoryDetail();
    $('#historyDetail').hidden = false;
    $('#historyDetail').scrollIntoView({ behavior: 'smooth', block: 'start' });
    updateHistoryButtons();
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

function renderHistoryDetail() {
  const result = state.history.detail;
  if (!result || !result.run) return;
  const run = result.run;
  $('#historyDetailTitle').textContent = `${run.host}:${run.port}`;
  $('#historyDetailKicker').textContent = `${protocolLabel(run.protocol)} · ${directionLabel(run.direction)} · ${localDateTime(run.createdAt)}`;
  setPill($('#historyDetailState'), run.status);
  renderMetricsInto($('#historyMetrics'), run.protocol, run.direction, run.summary, [], run.config ? run.config.durationSeconds : null);
  renderIntervalTableInto(
    $('#historyIntervalHead'),
    $('#historyIntervalRows'),
    result.intervals || [],
    run.protocol,
    run.direction,
    $('#historyIntervalNote'),
    `${result.intervalTotal || 0} 个区间 · 第 ${result.page || 1} 页`
  );
  const totalPages = Math.max(1, Math.ceil((result.intervalTotal || 0) / (result.pageSize || 200)));
  const page = Number(result.page || 1);
  $('#historyDetailPageLabel').textContent = `第 ${page} 页 / 共 ${totalPages} 页`;
  $('#historyPrevPage').disabled = page <= 1;
  $('#historyNextPage').disabled = page >= totalPages;
  const errorBox = $('#historyErrorBox');
  if (run.status === 'failed') {
    errorBox.textContent = result.stderrSummary
      ? `测试失败\n${result.stderrSummary}`
      : '测试失败，未保留 stderr 摘要';
    errorBox.hidden = false;
  } else {
    errorBox.hidden = true;
    errorBox.textContent = '';
  }
  let rawText = result.rawOutput || '';
  if (result.stderrSummary) {
    rawText += `${rawText ? '\n\n' : ''}--- stderr ---\n${result.stderrSummary}`;
  }
  $('#historyRawOutput').textContent = rawText;
  const lines = rawText ? rawText.split('\n').length : 0;
  $('#historyRawSummary').textContent = `iPerf3 原始输出${result.rawTruncated ? '（已截断）' : ''}${lines ? ` · ${lines} 行` : ''}`;
}

async function deleteSelectedHistory() {
  const ids = Array.from(state.history.selected);
  if (!ids.length) return;
  if (!window.confirm(`确定删除选中的 ${ids.length} 条记录？此操作不可恢复。`)) return;
  try {
    const result = await call('DeleteHistory', ids);
    if (result.error) {
      showToast(result.error.message || '删除失败', 'error');
      return;
    }
    showToast(`已删除 ${result.deleted} 条记录`, 'success');
    state.history.selected.clear();
    state.history.detail = null;
    state.history.detailPage = 1;
    $('#historyDetail').hidden = true;
    await loadHistory(state.history.page);
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

async function clearHistory() {
  if (!window.confirm('确定清空全部历史记录？此操作不可恢复。')) return;
  try {
    const result = await call('ClearHistory');
    if (result.error) {
      showToast(result.error.message || '清空失败', 'error');
      return;
    }
    showToast(`已清空 ${result.deleted} 条记录`, 'success');
    state.history.selected.clear();
    state.history.detail = null;
    state.history.detailPage = 1;
    $('#historyDetail').hidden = true;
    await loadHistory(1);
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

async function exportRecord(id, format) {
  try {
    const result = await call('ExportResult', id, format);
    if (result.error) {
      showToast(result.error.message || '导出失败', 'error');
      return;
    }
    if (result.canceled) return;
    showToast(`已导出到 ${result.path}`, 'success');
  } catch (error) {
    showToast(error.message || String(error), 'error');
  }
}

async function copyText(text, label) {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.append(textarea);
      textarea.select();
      document.execCommand('copy');
      textarea.remove();
    }
    showToast(`${label}已复制`, 'success');
  } catch {
    showToast('复制失败，请手动选择文本', 'error');
  }
}

function setView(view) {
  state.view = view;
  $$('.nav-tab').forEach((tab) => {
    tab.setAttribute('aria-selected', String(tab.dataset.view === view));
  });
  $$('.view').forEach((section) => {
    section.hidden = section.dataset.viewId !== view;
  });
  if (view === 'client' && state.runId) {
    refreshSnapshot();
  }
  if (view === 'history') {
    loadHistory(state.history.page);
  }
}

function bindEvents() {
  $$('.nav-tab').forEach((tab) => {
    tab.addEventListener('click', () => setView(tab.dataset.view));
  });
  $$('.segment-button[data-protocol]').forEach((button) => {
    button.addEventListener('click', () => {
      state.protocol = button.dataset.protocol;
      updateProtocolUI();
    });
  });
  $$('.segment-button[data-direction]').forEach((button) => {
    button.addEventListener('click', () => setDirection(button.dataset.direction));
  });
  ['host', 'port', 'duration', 'interval', 'parallel', 'bandwidth', 'bandwidthUnit', 'udpLength', 'bindAddress', 'ipVersion', 'interface'].forEach((id) => {
    const element = $(`#${id}`);
    if (element) {
      element.addEventListener('input', schedulePreview);
      element.addEventListener('change', schedulePreview);
    }
  });
  $('#startTest').addEventListener('click', startTest);
  $('#stopTest').addEventListener('click', stopTest);
  $('#exportCurrent').addEventListener('click', () => {
    if (state.historyId) exportRecord(state.historyId, 'json');
  });
  $('#copyCommand').addEventListener('click', () => copyText($('#commandPreview').textContent, '命令'));

  $('#oneOffSwitch').addEventListener('click', () => setOneOff(!state.server.oneOff));
  $('#serverPort').addEventListener('input', updateServerCommand);
  $('#serverBind').addEventListener('change', updateServerCommand);
  $('#startServer').addEventListener('click', startServer);
  $('#stopServer').addEventListener('click', stopServer);
  $('#copyServerCommand').addEventListener('click', () => copyText($('#serverCommand').textContent, '命令'));
  setOneOff(false);

  $('#historySearch').addEventListener('input', () => {
    clearTimeout(state.historySearchTimer);
    state.historySearchTimer = setTimeout(() => loadHistory(1), 250);
  });
  $('#historyProtocol').addEventListener('change', () => loadHistory(1));
  $('#historyDirection').addEventListener('change', () => loadHistory(1));
  $('#refreshHistory').addEventListener('click', () => loadHistory(state.history.page));
  $('#prevHistory').addEventListener('click', () => loadHistory(Math.max(1, state.history.page - 1)));
  $('#nextHistory').addEventListener('click', () => loadHistory(state.history.page + 1));
  $('#selectAllHistory').addEventListener('change', () => {
    const checked = $('#selectAllHistory').checked;
    state.history.selected.clear();
    if (checked) {
      for (const item of state.history.items) state.history.selected.add(item.id);
    }
    renderHistory();
  });
  $('#deleteHistory').addEventListener('click', deleteSelectedHistory);
  $('#clearHistory').addEventListener('click', clearHistory);
  $('#exportHistory').addEventListener('click', () => {
    const ids = Array.from(state.history.selected);
    if (ids.length === 1) {
      exportRecord(ids[0], 'csv');
      return;
    }
    if (state.history.detail) {
      exportRecord(state.history.detail.run.id, 'csv');
      return;
    }
    showToast('请选择一条记录或打开详情后再导出', 'error');
  });
  $('#historyExportJSON').addEventListener('click', () => {
    if (state.history.detail) exportRecord(state.history.detail.run.id, 'json');
  });
  $('#historyExportCSV').addEventListener('click', () => {
    if (state.history.detail) exportRecord(state.history.detail.run.id, 'csv');
  });
  $('#historyPrevPage').addEventListener('click', () => {
    if (state.history.detail) {
      loadHistoryDetail(state.history.detail.run.id, Math.max(1, state.history.detailPage - 1));
    }
  });
  $('#historyNextPage').addEventListener('click', () => {
    if (state.history.detail) {
      loadHistoryDetail(state.history.detail.run.id, state.history.detailPage + 1);
    }
  });
  $('#closeHistoryDetail').addEventListener('click', () => {
    state.history.detail = null;
    state.history.detailPage = 1;
    $('#historyDetail').hidden = true;
    updateHistoryButtons();
  });
}

async function init() {
  bindEvents();
  updateProtocolUI();
  updateDirectionHint();
  updateConfigKicker();
  renderMetrics();
  renderChart();
  renderIntervalTable();
  renderServer();
  try {
    await waitForRuntime();
    subscribeEvents();
    const status = await call('GetSystemStatus');
    applyEngineStatus(status);
    await loadHistory(1);
    await updateCommandPreview();
    await updateServerCommand();
  } catch (error) {
    $('#engineText').textContent = '运行时不可用';
    $('#engineDot').style.background = 'var(--red)';
    showToast(error.message || String(error), 'error');
  }
}

init();
