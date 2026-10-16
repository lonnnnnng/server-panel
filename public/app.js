const state = {
  metrics: null,
  connected: false,
  fallbackTimer: null
};

const els = {
  hostname: byId('hostname'),
  hostSubtitle: byId('hostSubtitle'),
  connectionDot: byId('connectionDot'),
  connectionText: byId('connectionText'),
  timestamp: byId('timestamp'),
  bootTimeBadge: byId('bootTimeBadge'),
  uptimeBadge: byId('uptimeBadge'),
  primaryAddress: byId('primaryAddress'),
  cpuValue: byId('cpuValue'),
  cpuRing: byId('cpuRing'),
  cpuRingValue: byId('cpuRingValue'),
  cpuLoad: byId('cpuLoad'),
  cpuCores: byId('cpuCores'),
  cpuSpeed: byId('cpuSpeed'),
  cpuTemp: byId('cpuTemp'),
  cpuUser: byId('cpuUser'),
  cpuSystem: byId('cpuSystem'),
  cpuIowait: byId('cpuIowait'),
  perCoreGrid: byId('perCoreGrid'),
  memoryValue: byId('memoryValue'),
  memoryUsedSegment: byId('memoryUsedSegment'),
  memoryCacheSegment: byId('memoryCacheSegment'),
  memoryUsed: byId('memoryUsed'),
  memoryAvailable: byId('memoryAvailable'),
  memoryCached: byId('memoryCached'),
  swapUsage: byId('swapUsage'),
  diskValue: byId('diskValue'),
  diskBar: byId('diskBar'),
  diskMount: byId('diskMount'),
  diskUsed: byId('diskUsed'),
  diskAvailable: byId('diskAvailable'),
  diskBusy: byId('diskBusy'),
  diskReadRate: byId('diskReadRate'),
  diskWriteRate: byId('diskWriteRate'),
  activeIfaces: byId('activeIfaces'),
  networkScope: byId('networkScope'),
  diskScope: byId('diskScope'),
  rxRate: byId('rxRate'),
  txRate: byId('txRate'),
  rxTotal: byId('rxTotal'),
  txTotal: byId('txTotal'),
  trafficTotal: byId('trafficTotal'),
  packetRate: byId('packetRate'),
  netErrorsDrops: byId('netErrorsDrops'),
  tcpEstablished: byId('tcpEstablished'),
  tcpListening: byId('tcpListening'),
  topCpuTable: byId('topCpuTable'),
  topMemoryTable: byId('topMemoryTable'),
  utilizationChart: byId('utilizationChart'),
  throughputChart: byId('throughputChart')
};

const charts = {
  utilization: els.utilizationChart.getContext('2d'),
  throughput: els.throughputChart.getContext('2d')
};

function byId(id) {
  return document.getElementById(id);
}

function clamp(value, min = 0, max = 100) {
  if (!Number.isFinite(value)) return min;
  return Math.max(min, Math.min(max, value));
}

function formatPercent(value, digits = 0) {
  if (!Number.isFinite(value)) return '--';
  return `${clamp(value).toFixed(digits)}%`;
}

function formatNumber(value, digits = 0) {
  if (!Number.isFinite(value)) return '--';
  return value.toLocaleString('zh-CN', {
    maximumFractionDigits: digits,
    minimumFractionDigits: digits
  });
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes)) return '--';
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  const digits = value >= 10 || index === 0 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[index]}`;
}

function formatRate(bytesPerSecond) {
  if (!Number.isFinite(bytesPerSecond)) return '--';
  return `${formatBytes(bytesPerSecond)}/s`;
}

function formatDuration(seconds) {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`;
  return `${Math.max(minutes, 0)} 分钟`;
}

function formatDateTime(value) {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  // long: 与刷新时间统一使用浏览器时区，避免服务器 UTC 时间被误认成本地开机时间。
  return date.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
  });
}

function formatTemp(celsius) {
  return Number.isFinite(celsius) ? `${celsius.toFixed(1)} C` : '--';
}

function setWidth(element, value) {
  element.style.width = `${clamp(value)}%`;
}

function setConnection(connected) {
  state.connected = connected;
  els.connectionDot.classList.toggle('online', connected);
  els.connectionDot.classList.toggle('offline', !connected);
  els.connectionText.textContent = connected ? '实时连接' : '连接中断';
}

function render(metrics) {
  state.metrics = metrics;
  setConnection(true);

  const primaryAddress = metrics.network.addresses?.[0];
  els.hostname.textContent = metrics.host.hostname;
  els.hostSubtitle.textContent = `${metrics.host.osName || metrics.host.platform} · ${metrics.host.arch} · ${metrics.cpu.cores} 核`;
  els.timestamp.textContent = new Date(metrics.timestamp).toLocaleTimeString('zh-CN', { hour12: false });
  els.bootTimeBadge.textContent = formatDateTime(metrics.host.bootTime);
  els.uptimeBadge.textContent = formatDuration(metrics.host.uptime);
  els.primaryAddress.textContent = primaryAddress ? `${primaryAddress.name} ${primaryAddress.address}` : '--';

  renderCpu(metrics);
  renderMemory(metrics.memory);
  renderDisk(metrics.disk);
  renderNetwork(metrics.network, metrics.tcp);
  renderProcesses(metrics.processes);
  drawUtilizationChart(metrics.history || []);
  drawThroughputChart(metrics.history || []);
}

function renderCpu(metrics) {
  const cpu = metrics.cpu;
  const percent = cpu.valid ? cpu.percent : null;
  els.cpuValue.textContent = formatPercent(percent);
  els.cpuRingValue.textContent = formatPercent(percent);
  els.cpuRing.style.setProperty('--value', clamp(percent).toFixed(1));
  els.cpuLoad.textContent = cpu.loadValid ? (cpu.load || []).map((value) => value.toFixed(2)).join(' / ') : '--';
  els.cpuCores.textContent = `${cpu.cores} 核`;
  els.cpuSpeed.textContent = cpu.speedMHz ? `${cpu.speedMHz} MHz` : '--';
  els.cpuTemp.textContent = formatTemp(metrics.temperature.cpu?.celsius);
  els.cpuUser.textContent = formatPercent(cpu.valid ? cpu.breakdown?.user : null);
  els.cpuSystem.textContent = formatPercent(cpu.valid ? cpu.breakdown?.system : null);
  els.cpuIowait.textContent = formatPercent(cpu.valid ? cpu.breakdown?.iowait : null);

  els.perCoreGrid.innerHTML = (cpu.perCore || [])
    .slice(0, 32)
    .map(
      (core) => `
        <div class="core-cell" title="CPU ${core.index}: ${formatPercent(cpu.valid ? core.percent : null)}">
          <span><i style="width: ${cpu.valid ? clamp(core.percent) : 0}%"></i></span>
          <strong>${core.index}</strong>
        </div>
      `
    )
    .join('');
}

function renderMemory(memory) {
  const percent = memory.valid ? memory.percent : null;
  // long: used 已经扣除 MemAvailable，缓存不能再次扣除，也不能当作互斥分区拼接。
  els.memoryValue.textContent = formatPercent(percent);
  setWidth(els.memoryUsedSegment, percent);
  setWidth(els.memoryCacheSegment, 0);
  els.memoryUsedSegment.parentElement.title = '已用 = 总量 − 可用；缓存独立展示，不与已用堆叠';
  els.memoryUsed.textContent = memory.valid ? `${formatBytes(memory.used)} / ${formatBytes(memory.total)}` : '--';
  els.memoryAvailable.textContent = memory.valid ? `${formatBytes(memory.available)}${memory.estimated ? '（估算）' : ''}` : '--';
  els.memoryCached.textContent = memory.valid ? formatBytes(memory.cached) : '--';
  els.swapUsage.textContent = !memory.valid ? '--' : memory.swapTotal > 0
    ? `${formatBytes(memory.swapUsed)} / ${formatBytes(memory.swapTotal)}`
    : '未启用';
}

function renderDisk(disk) {
  els.diskValue.textContent = formatPercent(disk.valid ? disk.percent : null);
  setWidth(els.diskBar, disk.valid ? disk.percent : 0);
  els.diskMount.textContent = disk.mount || '/';
  els.diskUsed.textContent = disk.valid ? `${formatBytes(disk.used)} / ${formatBytes(disk.total)}` : '--';
  els.diskAvailable.textContent = disk.valid ? formatBytes(disk.available) : '--';
  const validIO = disk.io?.valid && disk.io?.rateValid;
  els.diskBusy.textContent = formatPercent(validIO ? disk.io.busyPercent : null);
  els.diskReadRate.textContent = formatRate(validIO ? disk.io.readRate : null);
  els.diskWriteRate.textContent = formatRate(validIO ? disk.io.writeRate : null);
  els.diskScope.textContent = disk.io?.scope === 'whole-system-fallback' ? '整机 IO（逻辑设备回退，可能重复）' : '整机物理盘 IO（非根分区独占）';
}

function renderNetwork(network, tcp) {
  els.activeIfaces.textContent = network.valid ? `${network.selectedInterfaces?.length || 0} 口` : '--';
  els.networkScope.textContent = network.scope === 'physical' ? `物理网口：${network.selectedInterfaces?.join('、') || '无'}` : '虚拟接口回退：可能跨层重复计数';
  els.rxRate.textContent = formatRate(network.valid && network.rateValid ? network.rxRate : null);
  els.txRate.textContent = formatRate(network.valid && network.rateValid ? network.txRate : null);
  els.rxTotal.textContent = formatBytes(network.valid ? network.rxBytes : null);
  els.txTotal.textContent = formatBytes(network.valid ? network.txBytes : null);
  els.trafficTotal.textContent = formatBytes(network.valid ? network.rxBytes + network.txBytes : null);
  els.packetRate.textContent = network.valid && network.rateValid ? `${formatNumber(network.rxPacketRate)}/${formatNumber(network.txPacketRate)} pps` : '--';
  els.netErrorsDrops.textContent = network.valid ? `${formatNumber(network.errors)} / ${formatNumber(network.drops)}` : '--';
  els.tcpEstablished.textContent = tcp.valid ? formatNumber(tcp.established) : '--';
  els.tcpListening.textContent = tcp.valid ? `${formatNumber(tcp.listening)} TCP / ${formatNumber(tcp.udpListening)} UDP${tcp.partial ? '（部分）' : ''}` : '--';
}

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function renderProcesses(processes) {
  const data = processes || {};
  renderProcessTable(els.topCpuTable, data.topCpu || [], 'cpu');
  renderProcessTable(els.topMemoryTable, data.topMemory || [], 'memory');
}

function renderProcessTable(target, rows, mode) {
  if (!rows.length) {
    target.innerHTML = `<tr><td colspan="${mode === 'cpu' ? 7 : 8}">暂无数据</td></tr>`;
    return;
  }
  target.innerHTML = rows.map((process) => {
    const title = escapeHtml(process.command || process.name);
    const name = escapeHtml(process.name);
    if (mode === 'cpu') {
      return `<tr><td>${process.pid}</td><td title="${escapeHtml(process.user || process.uid || '--')}">${escapeHtml(process.user || process.uid || '--')}</td><td title="${title}">${name}</td><td>${formatRawPercent(process.cpuValid ? process.cpu : null, 1)}</td><td>${formatBytes(process.rss)}</td><td>${process.threads || 0}</td><td>${process.ppid || 0}</td></tr>`;
    }
    return `<tr><td>${process.pid}</td><td title="${title}">${name}</td><td>${formatBytes(process.rss)}</td><td>${formatRawPercent(process.memoryPercent || 0, 1)}</td><td>${formatBytes(process.vsz || 0)}</td><td>${formatBytes(process.rssAnon || 0)}</td><td>${formatBytes(process.rssFile || 0)}</td><td>${formatBytes(process.vmSwap || 0)}</td></tr>`;
  }).join('');
}

function formatRawPercent(value, digits = 0) {
  if (!Number.isFinite(value)) return '--';
  return `${Math.max(value, 0).toFixed(digits)}%`;
}

function drawUtilizationChart(history) {
  drawChart({ canvas: els.utilizationChart, ctx: charts.utilization, history, mode: 'percent', series: [
    { key: 'cpu', color: '#4fd1c5' }, { key: 'memory', color: '#f6c453' },
    { key: 'disk', color: '#f07167' }, { key: 'load', color: '#b8a1ff' }
  ] });
}

function drawThroughputChart(history) {
  drawChart({ canvas: els.throughputChart, ctx: charts.throughput, history, mode: 'bytes', series: [
    { key: 'rxRate', color: '#4fd1c5' }, { key: 'txRate', color: '#8ab4ff' },
    { key: 'readRate', color: '#b8a1ff' }, { key: 'writeRate', color: '#f07167' }
  ] });
}

function drawChart({ canvas, ctx, history, mode, series }) {
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  const width = Math.max(320, Math.floor(rect.width * dpr));
  const height = Math.max(220, Math.floor(rect.height * dpr));
  if (canvas.width !== width || canvas.height !== height) {
    canvas.width = width;
    canvas.height = height;
  }
  ctx.clearRect(0, 0, width, height);
  ctx.fillStyle = '#101413';
  ctx.fillRect(0, 0, width, height);
  const padding = { top: 18 * dpr, right: 18 * dpr, bottom: 24 * dpr, left: 48 * dpr };
  const innerWidth = width - padding.left - padding.right;
  const innerHeight = height - padding.top - padding.bottom;
  const maxValue = getChartMax(history, mode, series);
  ctx.strokeStyle = 'rgba(243, 245, 241, 0.10)';
  ctx.lineWidth = 1 * dpr;
  ctx.fillStyle = 'rgba(243, 245, 241, 0.46)';
  ctx.font = `${11 * dpr}px Inter, sans-serif`;
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let i = 0; i <= 4; i += 1) {
    const y = padding.top + (innerHeight / 4) * i;
    const value = maxValue - (maxValue / 4) * i;
    ctx.beginPath();
    ctx.moveTo(padding.left, y);
    ctx.lineTo(width - padding.right, y);
    ctx.stroke();
    ctx.fillText(mode === 'bytes' ? formatRate(value).replace('/s', '') : `${Math.round(value)}%`, padding.left - 8 * dpr, y);
  }
  for (const item of series) drawLine(ctx, history, item.key, item.color, padding, innerWidth, innerHeight, maxValue, dpr);
}

function getChartMax(history, mode, series) {
  const max = history.reduce((peak, point) => {
    const local = series.reduce((inner, item) => Math.max(inner, point[item.key] || 0), 0);
    return Math.max(peak, local);
  }, 1);
  if (mode === 'percent' && max <= 100) return 100;
  const magnitude = 10 ** Math.floor(Math.log10(max));
  return Math.ceil(max / magnitude) * magnitude;
}

function drawLine(ctx, history, key, color, padding, innerWidth, innerHeight, maxValue, dpr) {
  if (!history.length) return;
  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.lineWidth = 2.2 * dpr;
  ctx.lineJoin = 'round';
  ctx.lineCap = 'round';
  let hasPrevious = false;
  history.forEach((point, index) => {
    if (!Number.isFinite(point[key])) {
      hasPrevious = false;
      return;
    }
    const x = padding.left + (history.length === 1 ? innerWidth : (innerWidth / (history.length - 1)) * index);
    const value = clamp(point[key], 0, maxValue);
    const y = padding.top + innerHeight - (value / maxValue) * innerHeight;
    if (!hasPrevious) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
    hasPrevious = true;
  });
  ctx.stroke();
}

async function pollOnce() {
  const response = await fetch('/api/metrics', { cache: 'no-store' });
  if (!response.ok) throw new Error(`metrics request failed: ${response.status}`);
  render(await response.json());
}

window.addEventListener('resize', () => {
  if (!state.metrics) return;
  drawUtilizationChart(state.metrics.history || []);
  drawThroughputChart(state.metrics.history || []);
});

function startFallbackPolling() {
  if (state.fallbackTimer) return;
  state.fallbackTimer = setInterval(() => {
    pollOnce().catch(() => setConnection(false));
  }, 1000);
}

function startStream() {
  if (!window.EventSource) {
    startFallbackPolling();
    return;
  }

  const stream = new EventSource('/api/events');
  stream.onmessage = (event) => {
    // long: SSE 恢复后只保留推送，避免轮询与推送叠加产生重复请求和重绘。
    if (state.fallbackTimer) {
      clearInterval(state.fallbackTimer);
      state.fallbackTimer = null;
    }
    render(JSON.parse(event.data));
  };
  stream.onerror = () => {
    setConnection(false);
    startFallbackPolling();
  };
}

pollOnce().catch(() => setConnection(false));
startStream();
