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
  rxRate: byId('rxRate'),
  txRate: byId('txRate'),
  rxTotal: byId('rxTotal'),
  txTotal: byId('txTotal'),
  trafficTotal: byId('trafficTotal'),
  packetRate: byId('packetRate'),
  netErrorsDrops: byId('netErrorsDrops'),
  tcpEstablished: byId('tcpEstablished'),
  tcpListening: byId('tcpListening'),
  pressureCpu: byId('pressureCpu'),
  pressureMemory: byId('pressureMemory'),
  pressureIo: byId('pressureIo'),
  fileHandles: byId('fileHandles'),
  processSummary: byId('processSummary'),
  tcpTotal: byId('tcpTotal'),
  portListeners: byId('portListeners'),
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
  return `${clamp(value).toFixed(digits)}%`;
}

function formatRawPercent(value, digits = 0) {
  if (!Number.isFinite(value)) return '--';
  return `${Math.max(value, 0).toFixed(digits)}%`;
}

function formatNumber(value, digits = 0) {
  if (!Number.isFinite(value)) return '--';
  return value.toLocaleString('zh-CN', {
    maximumFractionDigits: digits,
    minimumFractionDigits: digits
  });
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
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
  els.uptimeBadge.textContent = formatDuration(metrics.host.uptime);
  els.primaryAddress.textContent = primaryAddress ? `${primaryAddress.name} ${primaryAddress.address}` : '--';

  renderCpu(metrics);
  renderMemory(metrics.memory);
  renderDisk(metrics.disk);
  renderNetwork(metrics.network, metrics.tcp);
  renderSummary(metrics);
  // long: 端口占用独立于网络详情展示，删除详情区后仍需持续刷新监听端口。
  renderPortListeners(metrics.tcp.listeners || []);
  renderProcesses(metrics.processes);
  drawUtilizationChart(metrics.history || []);
  drawThroughputChart(metrics.history || []);
}

function renderCpu(metrics) {
  const cpu = metrics.cpu;
  const percent = cpu.percent || 0;
  els.cpuValue.textContent = formatPercent(percent);
  els.cpuRingValue.textContent = formatPercent(percent);
  els.cpuRing.style.setProperty('--value', percent.toFixed(1));
  els.cpuLoad.textContent = (cpu.load || []).map((value) => value.toFixed(2)).join(' / ');
  els.cpuCores.textContent = `${cpu.cores} 核`;
  els.cpuSpeed.textContent = cpu.speedMHz ? `${cpu.speedMHz} MHz` : '--';
  els.cpuTemp.textContent = formatTemp(metrics.temperature.cpu?.celsius ?? metrics.temperature.max);
  els.cpuUser.textContent = formatPercent(cpu.breakdown?.user || 0);
  els.cpuSystem.textContent = formatPercent(cpu.breakdown?.system || 0);
  els.cpuIowait.textContent = formatPercent(cpu.breakdown?.iowait || 0);

  els.perCoreGrid.innerHTML = (cpu.perCore || [])
    .slice(0, 32)
    .map(
      (core) => `
        <div class="core-cell" title="CPU ${core.index}: ${formatPercent(core.percent)}">
          <span><i style="width: ${clamp(core.percent)}%"></i></span>
          <strong>${core.index}</strong>
        </div>
      `
    )
    .join('');
}

function renderMemory(memory) {
  const percent = memory.percent || 0;
  const appUsed = Math.max((memory.used || 0) - (memory.cached || 0) - (memory.buffers || 0), 0);
  const appUsedPct = memory.total > 0 ? (appUsed / memory.total) * 100 : 0;
  const cachePct = memory.total > 0 ? ((memory.cached || 0) / memory.total) * 100 : 0;

  els.memoryValue.textContent = formatPercent(percent);
  setWidth(els.memoryUsedSegment, appUsedPct);
  setWidth(els.memoryCacheSegment, Math.min(cachePct, 100 - appUsedPct));
  els.memoryUsed.textContent = `${formatBytes(memory.used)} / ${formatBytes(memory.total)}`;
  els.memoryAvailable.textContent = formatBytes(memory.available);
  els.memoryCached.textContent = formatBytes(memory.cached);
  els.swapUsage.textContent = memory.swapTotal > 0
    ? `${formatBytes(memory.swapUsed)} / ${formatBytes(memory.swapTotal)}`
    : '未启用';
}

function renderDisk(disk) {
  els.diskValue.textContent = formatPercent(disk.percent || 0);
  setWidth(els.diskBar, disk.percent || 0);
  els.diskMount.textContent = disk.mount || '/';
  els.diskUsed.textContent = `${formatBytes(disk.used)} / ${formatBytes(disk.total)}`;
  els.diskAvailable.textContent = formatBytes(disk.available);
  els.diskBusy.textContent = formatPercent(disk.io?.busyPercent || 0);
  els.diskReadRate.textContent = formatRate(disk.io?.readRate || 0);
  els.diskWriteRate.textContent = formatRate(disk.io?.writeRate || 0);
}

function renderNetwork(network, tcp) {
  els.activeIfaces.textContent = `${network.interfaces?.length || 0} 张`;
  els.rxRate.textContent = formatRate(network.rxRate || 0);
  els.txRate.textContent = formatRate(network.txRate || 0);
  els.rxTotal.textContent = formatBytes(network.rxBytes || 0);
  els.txTotal.textContent = formatBytes(network.txBytes || 0);
  els.trafficTotal.textContent = formatBytes((network.rxBytes || 0) + (network.txBytes || 0));
  els.packetRate.textContent = `${formatNumber(network.rxPacketRate || 0)}/${formatNumber(network.txPacketRate || 0)} pps`;
  els.netErrorsDrops.textContent = `${formatNumber(network.errors || 0)} / ${formatNumber(network.drops || 0)}`;
  els.tcpEstablished.textContent = formatNumber(tcp.established || 0);
  els.tcpListening.textContent = `${formatNumber(tcp.listening || 0)} TCP / ${formatNumber(tcp.udpListening || 0)} UDP`;
}

function renderSummary(metrics) {
  els.pressureCpu.textContent = formatPressure(metrics.pressure.cpu);
  els.pressureMemory.textContent = formatPressure(metrics.pressure.memory);
  els.pressureIo.textContent = formatPressure(metrics.pressure.io);
  els.fileHandles.textContent = formatFileHandles(metrics.fileHandles);
  els.processSummary.textContent = `${formatNumber(metrics.processes.total)} / ${formatNumber(metrics.processes.running)}`;
  els.tcpTotal.textContent = formatNumber(metrics.tcp.total || 0);
}

function formatFileHandles(fileHandles) {
  if (!fileHandles) return '--';
  if (!Number.isFinite(fileHandles.max) || fileHandles.max > 1e12) {
    return `${formatNumber(fileHandles.used)} 已用`;
  }
  return `${formatNumber(fileHandles.used)} / ${formatNumber(fileHandles.max)}`;
}

function formatPressure(pressure) {
  const some = pressure?.some?.avg10;
  const full = pressure?.full?.avg10;
  if (!Number.isFinite(some) && !Number.isFinite(full)) return '--';
  if (Number.isFinite(some) && Number.isFinite(full) && full > 0) return `${some.toFixed(2)} / ${full.toFixed(2)}`;
  if (Number.isFinite(some)) return `${some.toFixed(2)}`;
  return `${full.toFixed(2)}`;
}

function renderPortListeners(listeners) {
  if (!listeners.length) {
    els.portListeners.innerHTML = '<tr><td colspan="7">未读取到监听端口</td></tr>';
    return;
  }

  els.portListeners.innerHTML = listeners
    .map((item) => {
      const command = item.command || item.process || '--';
      return `
        <tr>
          <td>${escapeHtml((item.protocol || '').toUpperCase())}</td>
          <td title="${escapeHtml(item.localAddress || '--')}">${escapeHtml(formatListenAddress(item.localAddress))}</td>
          <td>${formatNumber(item.port || 0)}</td>
          <td>${item.pid || '--'}</td>
          <td title="${escapeHtml(item.user || '--')}">${escapeHtml(item.user || '--')}</td>
          <td title="${escapeHtml(item.process || '--')}">${escapeHtml(item.process || '--')}</td>
          <td title="${escapeHtml(command)}">${escapeHtml(command)}</td>
        </tr>
      `;
    })
    .join('');
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

  target.innerHTML = rows
    .map((process) => {
      const title = escapeHtml(process.command || process.name);
      const name = escapeHtml(process.name);
      if (mode === 'cpu') {
        return `
          <tr>
            <td>${process.pid}</td>
            <td title="${escapeHtml(process.user || process.uid || '--')}">${escapeHtml(process.user || process.uid || '--')}</td>
            <td title="${title}">${name}</td>
            <td>${formatRawPercent(process.cpu || 0, 1)}</td>
            <td>${formatBytes(process.rss)}</td>
            <td>${process.threads || 0}</td>
            <td>${process.ppid || 0}</td>
          </tr>
        `;
      }
      return `
        <tr>
          <td>${process.pid}</td>
          <td title="${title}">${name}</td>
          <td>${formatBytes(process.rss)}</td>
          <td>${formatRawPercent(process.memoryPercent || 0, 1)}</td>
          <td>${formatBytes(process.vsz || 0)}</td>
          <td>${formatBytes(process.rssAnon || 0)}</td>
          <td>${formatBytes(process.rssFile || 0)}</td>
          <td>${formatBytes(process.vmSwap || 0)}</td>
        </tr>
      `;
    })
    .join('');
}

function formatListenAddress(address) {
  if (address === '0.0.0.0' || address === '::') return '全部地址';
  return address || '--';
}

function escapeHtml(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function drawUtilizationChart(history) {
  drawChart({
    canvas: els.utilizationChart,
    ctx: charts.utilization,
    history,
    mode: 'percent',
    series: [
      { key: 'cpu', color: '#4fd1c5' },
      { key: 'memory', color: '#f6c453' },
      { key: 'disk', color: '#f07167' },
      { key: 'load', color: '#b8a1ff' }
    ]
  });
}

function drawThroughputChart(history) {
  drawChart({
    canvas: els.throughputChart,
    ctx: charts.throughput,
    history,
    mode: 'bytes',
    series: [
      { key: 'rxRate', color: '#4fd1c5' },
      { key: 'txRate', color: '#8ab4ff' },
      { key: 'readRate', color: '#b8a1ff' },
      { key: 'writeRate', color: '#f07167' }
    ]
  });
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

  const padding = {
    top: 18 * dpr,
    right: 18 * dpr,
    bottom: 24 * dpr,
    left: 48 * dpr
  };
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

  for (const item of series) {
    drawLine(ctx, history, item.key, item.color, padding, innerWidth, innerHeight, maxValue, dpr);
  }
}

function getChartMax(history, mode, series) {
  if (mode === 'percent') return 100;
  const max = history.reduce((peak, point) => {
    const local = series.reduce((inner, item) => Math.max(inner, point[item.key] || 0), 0);
    return Math.max(peak, local);
  }, 1);
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

  history.forEach((point, index) => {
    const x = padding.left + (history.length === 1 ? innerWidth : (innerWidth / (history.length - 1)) * index);
    const value = clamp(point[key] || 0, 0, maxValue);
    const y = padding.top + innerHeight - (value / maxValue) * innerHeight;
    if (index === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.stroke();
}

async function pollOnce() {
  const response = await fetch('/api/metrics', { cache: 'no-store' });
  if (!response.ok) throw new Error(`metrics request failed: ${response.status}`);
  render(await response.json());
}

function startFallbackPolling() {
  if (state.fallbackTimer) return;
  state.fallbackTimer = setInterval(() => {
    pollOnce().catch(() => setConnection(false));
  }, 2000);
}

function startStream() {
  if (!window.EventSource) {
    startFallbackPolling();
    return;
  }

  const stream = new EventSource('/api/events');
  stream.onmessage = (event) => render(JSON.parse(event.data));
  stream.onerror = () => {
    setConnection(false);
    startFallbackPolling();
  };
}

window.addEventListener('resize', () => {
  if (!state.metrics) return;
  drawUtilizationChart(state.metrics.history || []);
  drawThroughputChart(state.metrics.history || []);
});

pollOnce().catch(() => setConnection(false));
startStream();
