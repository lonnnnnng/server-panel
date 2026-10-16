const assert = require('node:assert/strict');

// long: 验收只输出汇总，不打印完整指标，避免进程命令行中的凭据进入日志。
async function main() {
  const base = new URL(process.argv[2] || 'http://127.0.0.1:8888');
  const health = await fetch(new URL('/api/health', base), {signal:AbortSignal.timeout(5000)}).then(r => r.json());
  assert.equal(health.ok, true);
  const response = await fetch(new URL('/api/events', base), {signal:AbortSignal.timeout(12000)});
  assert.equal(response.ok, true);
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = '';
  const samples = [];
  try {
    while (samples.length < 12) {
      const {value,done} = await reader.read();
      assert.equal(done, false, 'SSE ended before enough samples');
      pending += decoder.decode(value, {stream:true});
      let end;
      while ((end = pending.indexOf('\n\n')) >= 0) {
        const frame = pending.slice(0,end);
        pending = pending.slice(end+2);
        const data = frame.split('\n').find(line => line.startsWith('data: '));
        if (!data) continue;
        const m = JSON.parse(data.slice(6));
        if (samples.at(-1)?.timestamp !== m.timestamp) samples.push(m);
      }
    }
  } finally {
    await reader.cancel();
  }
  const sum = (items,key) => items.reduce((n,item) => n + item[key], 0);
  const near = (a,b,label,tolerance=1e-6) => assert.ok(Math.abs(a-b) <= tolerance, `${label}: ${a} != ${b}`);
  for (const m of samples) {
    assert.ok(m.cpu.valid && m.memory.valid && m.disk.valid && m.network.valid, 'core source unavailable');
    assert.equal(m.memory.used, m.memory.total - m.memory.available);
    near(m.memory.percent, m.memory.used/m.memory.total*100, 'memory percent', 0.001);
    assert.equal(m.disk.percent, Math.ceil(m.disk.used/(m.disk.used+m.disk.available)*100));
    near(Object.values(m.cpu.breakdown).reduce((a,b)=>a+b,0), 100, 'CPU breakdown', 0.01);
    assert.equal(m.hardware.cpu.speedMHz, m.cpu.speedMHz);
    assert.equal(m.hardware.memory.available, m.memory.available);
    assert.equal(m.hardware.memory.swapTotal, m.memory.swapTotal);
    const selected = m.network.interfaces.filter(x => m.network.selectedInterfaces.includes(x.name));
    for (const key of ['rxBytes','txBytes','rxPackets','txPackets','rxErrors','txErrors','rxDrops','txDrops','rxRate','txRate']) {
      near(m.network[key],sum(selected,key),`network ${key}`);
    }
    assert.equal(m.tcp.listeners.length, m.tcp.listening + m.tcp.udpListening);
    const h = m.history.at(-1);
    assert.equal(h.timestamp,m.timestamp);
    assert.equal(h.cpu,m.cpu.percent);
    assert.equal(h.memory,m.memory.percent);
    assert.equal(h.rxRate,m.network.rxRate);
  }
  const times = samples.map(x=>new Date(x.timestamp).getTime());
  const intervals = times.slice(1).map((t,i)=>t-times[i]);
  const m = samples.at(-1);
  console.log(JSON.stringify({health,samples:samples.length,intervalMs:{min:Math.min(...intervals),max:Math.max(...intervals)},
    memoryPercent:m.memory.percent,diskPercent:m.disk.percent,networkScope:m.network.scope,interfaces:m.network.selectedInterfaces,
    ioScope:m.disk.io.scope,tcpListeners:m.tcp.listening,udpListeners:m.tcp.udpListening,
    historyCurrent:true,processCpuAvailable:m.processes.topCpu.some(x=>x.cpuValid)},null,2));
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
