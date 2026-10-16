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
  hostGroups: byId('hostGroups'),
  hardware: byId('hardware'),
  localAddresses: byId('localAddresses'),
  tempZones: byId('tempZones'),
  fsList: byId('fsList'),
  memoryDetails: byId('memoryDetails'),
  interfaces: byId('interfaces'),
  tcpStates: byId('tcpStates'),
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
  renderHost(metrics);
  renderHardware(metrics.hardware || {});
  renderFilesystems(metrics.disk.filesystems || []);
  renderMemoryDetails(metrics.memory || {});
  renderInterfaces(metrics.network.interfaces || []);
  renderTcp(metrics.tcp);
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

function renderHost(metrics) {
  const host = metrics.host || {};
  const groups = [
    {
      title: '系统',
      rows: [
        ['主机名', host.hostname],
        ['系统', host.osName || host.platform],
        ['发行版', joinValues([host.osId, host.osVersionId, host.osVersionCodename])],
        ['内核', host.release],
        ['内核构建', host.kernelVersion],
        ['架构', joinValues([host.arch, host.machine])],
        ['Go 运行时', host.goVersion],
        ['CPU 型号', metrics.cpu.model]
      ]
    },
    {
      title: '运行',
      rows: [
        ['运行时长', formatDuration(host.uptime || 0)],
        ['启动时间', formatDateTime(host.bootTime)],
        ['本地时间', formatDateTime(host.localTime)],
        ['时区', host.timezone],
        ['虚拟化', host.virtualization],
        ['容器', host.container]
      ]
    },
    {
      title: '用户',
      rows: [
        ['当前用户', host.user],
        ['UID / GID', joinValues([host.uid, host.gid], ' / ')],
        ['Home', host.homeDir],
        ['Shell', host.shell]
      ]
    },
    {
      title: '硬件',
      rows: [
        ['厂商', host.productVendor],
        ['型号', host.productName],
        ['版本', host.productVersion],
        ['系列', host.productFamily],
        ['主板厂商', host.boardVendor],
        ['主板型号', host.boardName],
        ['主板版本', host.boardVersion],
        ['机箱厂商', host.chassisVendor],
        ['机箱', host.chassisType],
        ['机箱版本', host.chassisVersion]
      ]
    },
    {
      title: 'BIOS',
      rows: [
        ['厂商', host.biosVendor],
        ['版本', host.biosVersion],
        ['日期', host.biosDate]
      ]
    }
  ];

  els.hostGroups.innerHTML = groups
    .map(renderHostGroup)
    .filter(Boolean)
    .join('');

  els.localAddresses.innerHTML = (metrics.network.addresses || [])
    .map((item) => `<span>${escapeHtml(item.name)} ${escapeHtml(item.address)}</span>`)
    .join('');

  const zones = metrics.temperature.zones || [];
  els.tempZones.innerHTML = zones.length
    ? zones
        .map(
          (zone) => `
            <div>
              <span>${escapeHtml(zone.type)}</span>
              <strong>${formatTemp(zone.celsius)}</strong>
            </div>
          `
        )
        .join('')
    : '<div><span>温度传感器</span><strong>--</strong></div>';
}

function renderHostGroup(group) {
  const rows = group.rows.filter(([, value]) => hasValue(value));
  if (!rows.length) return '';
  return `
    <section class="host-section">
      <h3>${escapeHtml(group.title)}</h3>
      <dl class="info-list host-info-list">
        ${rows
          .map(
            ([label, value]) => `
              <div>
                <dt>${escapeHtml(label)}</dt>
                <dd title="${escapeHtml(value)}">${escapeHtml(value)}</dd>
              </div>
            `
          )
          .join('')}
      </dl>
    </section>
  `;
}

function renderHardware(hardware) {
  const cpu = hardware.cpu || {};
  const memory = hardware.memory || {};
  const motherboard = hardware.motherboard || {};
  const flags = cpu.flags || [];
  const visibleFlags = flags.slice(0, 18);
  const flagSuffix = flags.length > visibleFlags.length ? `+${flags.length - visibleFlags.length}` : '';

  const groups = [
    renderHardwareInfoGroup('主板', [
      ['整机厂商', motherboard.systemVendor],
      ['整机型号', motherboard.systemModel],
      ['整机版本', motherboard.systemVersion],
      ['产品系列', motherboard.productFamily],
      ['主板厂商', motherboard.boardVendor],
      ['主板型号', motherboard.boardName],
      ['主板版本', motherboard.boardVersion],
      ['BIOS 厂商', motherboard.biosVendor],
      ['BIOS 版本', motherboard.biosVersion],
      ['BIOS 日期', motherboard.biosDate],
      ['机箱厂商', motherboard.chassisVendor],
      ['机箱类型', motherboard.chassisType],
      ['机箱版本', motherboard.chassisVersion]
    ]),
    renderHardwareInfoGroup('CPU', [
      ['型号', cpu.model],
      ['厂商', cpu.vendor],
      ['架构', cpu.architecture],
      ['逻辑核心', cpu.logicalCores ? `${cpu.logicalCores} 核` : ''],
      ['物理核心', cpu.physicalCores ? `${cpu.physicalCores} 核` : ''],
      ['Socket', cpu.sockets ? `${cpu.sockets}` : ''],
      ['每 Socket 核心', cpu.coresPerSocket ? `${cpu.coresPerSocket}` : ''],
      ['当前频率', cpu.speedMHz ? `${cpu.speedMHz} MHz` : ''],
      ['最高频率', cpu.maxMHz ? `${cpu.maxMHz} MHz` : ''],
      ['最低频率', cpu.minMHz ? `${cpu.minMHz} MHz` : ''],
      ['缓存', cpu.cacheSize],
      ['BogoMIPS', cpu.bogomips],
      ['微码', cpu.microcode],
      ['特性', visibleFlags.length ? `${visibleFlags.join(' ')} ${flagSuffix}`.trim() : '']
    ]),
    renderHardwareInfoGroup('内存', [
      ['总量', formatBytes(memory.total || 0)],
      ['可用', formatBytes(memory.available || 0)],
      ['Swap 总量', memory.swapTotal ? formatBytes(memory.swapTotal) : '未启用'],
      ['Swap 可用', memory.swapTotal ? formatBytes(memory.swapFree || 0) : ''],
      ['页大小', memory.pageSize ? formatBytes(memory.pageSize) : ''],
      ['HugePage 大小', memory.hugePageSize ? formatBytes(memory.hugePageSize) : ''],
      ['HugePage 总数', memory.hugePagesTotal ? formatNumber(memory.hugePagesTotal) : ''],
      ['HugePage 空闲', memory.hugePagesFree ? formatNumber(memory.hugePagesFree) : ''],
      ['NUMA 节点', memory.numaNodes ? formatNumber(memory.numaNodes) : '']
    ]),
    renderHardwareDeviceGroup('硬盘', hardware.storage || [], renderStorageHardware),
    renderHardwareDeviceGroup('显卡', hardware.gpus || [], renderGPUHardware),
    renderHardwareDeviceGroup('网卡', hardware.network || [], renderNetworkHardware)
  ].filter(Boolean);

  els.hardware.innerHTML = groups.length
    ? groups.join('')
    : '<section class="hardware-section"><h3>硬件</h3><div class="hardware-empty">未读取到硬件详情</div></section>';
}

function renderHardwareInfoGroup(title, rows) {
  const visibleRows = rows.filter(([, value]) => hasValue(value));
  if (!visibleRows.length) return '';
  return `
    <section class="hardware-section">
      <h3>${escapeHtml(title)}</h3>
      <dl class="info-list hardware-info-list">
        ${visibleRows
          .map(
            ([label, value]) => `
              <div>
                <dt>${escapeHtml(label)}</dt>
                <dd title="${escapeHtml(value)}">${escapeHtml(value)}</dd>
              </div>
            `
          )
          .join('')}
      </dl>
    </section>
  `;
}

function renderHardwareDeviceGroup(title, items, renderer) {
  const rows = items.map(renderer).filter(Boolean);
  if (!rows.length) {
    return `
      <section class="hardware-section">
        <h3>${escapeHtml(title)}</h3>
        <div class="hardware-empty">未读取到${escapeHtml(title)}设备</div>
      </section>
    `;
  }
  return `
    <section class="hardware-section">
      <h3>${escapeHtml(title)}</h3>
      <div class="hardware-device-list">${rows.join('')}</div>
    </section>
  `;
}

function renderStorageHardware(item) {
  const title = joinValues([item.name, item.model]);
  const meta = joinValues([
    item.medium,
    item.transport ? item.transport.toUpperCase() : '',
    item.size ? formatBytes(item.size) : '',
    item.partitions ? `${item.partitions} 分区` : '',
    item.mountpoints?.length ? `挂载 ${item.mountpoints.join(', ')}` : '',
    item.filesystems?.length ? `文件系统 ${item.filesystems.join(', ')}` : '',
    item.removable ? '可移除' : ''
  ], ' · ');
  return renderHardwareDevice(title || '磁盘设备', meta);
}

function renderGPUHardware(item) {
  const title = joinValues([item.vendor, item.device]);
  const meta = joinValues([
    item.class,
    item.slot ? `PCI ${item.slot}` : '',
    item.subsystemVendor && item.subsystemDevice ? `${item.subsystemVendor} ${item.subsystemDevice}` : ''
  ], ' · ');
  return renderHardwareDevice(title || '显示设备', meta);
}

function renderNetworkHardware(item) {
  const title = joinValues([item.name, item.vendor, item.device]);
  const meta = joinValues([
    formatHardwareKind(item.kind),
    item.state,
    item.speedMbps ? `${item.speedMbps} Mbps` : '',
    item.duplex,
    item.mtu ? `MTU ${item.mtu}` : '',
    item.driver ? `驱动 ${item.driver}` : '',
    item.bus ? `PCI ${item.bus}` : '',
    item.pciId
  ], ' · ');
  return renderHardwareDevice(title || item.name || '网络设备', meta);
}

function renderHardwareDevice(title, meta) {
  return `
    <div class="hardware-device">
      <strong title="${escapeHtml(title)}">${escapeHtml(title)}</strong>
      <span title="${escapeHtml(meta)}">${escapeHtml(meta || '--')}</span>
    </div>
  `;
}

function formatHardwareKind(kind) {
  switch (kind) {
    case 'physical':
      return '物理';
    case 'virtual':
      return '虚拟';
    case 'virtual-active':
      return '虚拟活动';
    default:
      return kind || '';
  }
}

function hasValue(value) {
  return value !== null && value !== undefined && String(value).trim() !== '' && String(value).trim() !== '--';
}

function joinValues(values, separator = ' ') {
  return values.filter(hasValue).join(separator);
}

function formatDateTime(value) {
  if (!hasValue(value)) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { hour12: false });
}

function renderFilesystems(filesystems) {
  if (!filesystems.length) {
    els.fsList.innerHTML = '<div class="fs-row"><div class="fs-title"><strong>未读取到文件系统</strong><span>--</span></div></div>';
    return;
  }

  els.fsList.innerHTML = filesystems
    .map(
      (fs) => `
        <div class="fs-row">
          <div class="fs-title">
            <strong title="${escapeHtml(fs.mount)}">${escapeHtml(fs.mount)}</strong>
            <span>${formatPercent(fs.percent)}</span>
          </div>
          <div class="mini-bar"><span style="width: ${clamp(fs.percent)}%"></span></div>
          <div class="fs-meta">${escapeHtml(fs.type)} · ${formatBytes(fs.used)} / ${formatBytes(fs.total)} · ${escapeHtml(fs.filesystem)}</div>
        </div>
      `
    )
    .join('');
}

function renderMemoryDetails(memory) {
  const appUsed = Math.max((memory.used || 0) - (memory.cached || 0) - (memory.buffers || 0), 0);
  const rows = [
    ['应用占用', formatBytes(appUsed)],
    ['缓存', formatBytes(memory.cached || 0)],
    ['Buffers', formatBytes(memory.buffers || 0)],
    ['Slab', formatBytes(memory.slab || 0)],
    ['可回收 Slab', formatBytes(memory.sReclaimable || 0)],
    ['Active', formatBytes(memory.active || 0)],
    ['Inactive', formatBytes(memory.inactive || 0)],
    ['Dirty', formatBytes(memory.dirty || 0)],
    ['Writeback', formatBytes(memory.writeback || 0)],
    ['匿名页', formatBytes(memory.anonPages || 0)],
    ['Mapped', formatBytes(memory.mapped || 0)],
    ['Shmem', formatBytes(memory.shmem || 0)],
    ['页表', formatBytes(memory.pageTables || 0)],
    ['Commit', `${formatBytes(memory.committedAs || 0)} / ${formatBytes(memory.commitLimit || 0)}`],
    ['Swap', memory.swapTotal > 0 ? `${formatBytes(memory.swapUsed || 0)} / ${formatBytes(memory.swapTotal || 0)}` : '未启用']
  ];

  els.memoryDetails.innerHTML = rows
    .map(
      ([label, value]) => `
        <div>
          <span>${escapeHtml(label)}</span>
          <strong title="${escapeHtml(value)}">${escapeHtml(value)}</strong>
        </div>
      `
    )
    .join('');
}

function renderInterfaces(interfaces) {
  if (!interfaces.length) {
    els.interfaces.innerHTML = '<div class="interface-row"><div class="interface-title"><strong>无活动网卡</strong><span>--</span></div></div>';
    return;
  }

  const maxRate = Math.max(...interfaces.map((item) => item.rxRate + item.txRate), 1);
  els.interfaces.innerHTML = interfaces
    .map((item) => {
      const totalRate = item.rxRate + item.txRate;
      return `
        <div class="interface-row">
          <div class="interface-title">
            <strong title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</strong>
            <span>${formatRate(totalRate)}</span>
          </div>
          <div class="mini-bar"><span style="width: ${clamp((totalRate / maxRate) * 100)}%"></span></div>
          <div class="interface-meta">
            ${escapeHtml(formatInterfaceKind(item.kind))} · ${escapeHtml(item.state || '--')} · ${item.speedMbps ? `${formatNumber(item.speedMbps)} Mbps` : '速率 --'} · ${escapeHtml(item.duplex || '双工 --')} · MTU ${formatNumber(item.mtu || 0)}
          </div>
          <div class="interface-meta">
            RX ${formatRate(item.rxRate)} / ${formatBytes(item.rxBytes || 0)} · TX ${formatRate(item.txRate)} / ${formatBytes(item.txBytes || 0)} · 包 ${formatNumber(item.rxPacketRate || 0)}/${formatNumber(item.txPacketRate || 0)} pps
          </div>
          <div class="interface-meta">
            错误 ${formatNumber((item.rxErrors || 0) + (item.txErrors || 0))} · 丢包 ${formatNumber((item.rxDrops || 0) + (item.txDrops || 0))}${item.driver ? ` · 驱动 ${escapeHtml(item.driver)}` : ''}
          </div>
        </div>
      `;
    })
    .join('');
}

function renderTcp(tcp) {
  const states = tcp.states || {};
  const rows = [
    ['ESTABLISHED', states.established || 0],
    ['TIME_WAIT', states.timeWait || 0],
    ['CLOSE_WAIT', states.closeWait || 0],
    ['SYN_RECV', states.synRecv || 0],
    ['TCP LISTEN', states.listening || 0],
    ['UDP 端口', tcp.udpListening || 0]
  ];

  els.tcpStates.innerHTML = rows
    .map(
      ([name, value]) => `
        <div>
          <span>${name}</span>
          <strong>${formatNumber(value)}</strong>
        </div>
      `
    )
    .join('');

  renderPortListeners(tcp.listeners || []);
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

function formatInterfaceKind(kind) {
  switch (kind) {
    case 'physical':
      return '物理';
    case 'virtual':
      return '虚拟';
    case 'virtual-active':
      return '虚拟活动';
    default:
      return kind || '--';
  }
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
