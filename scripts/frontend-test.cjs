const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// long: 用最小 DOM 执行实际前端源码，覆盖计算与缺失数据，不依赖额外浏览器或 npm 包。
const elements = new Map();
const element = (id) => {
  if (!elements.has(id)) elements.set(id, {
    textContent: '', innerHTML: '', style: { setProperty() {} }, parentElement: {},
    classList: { toggle() {} }, getContext: () => ({}),
  });
  return elements.get(id);
};
let stream;
let cleared = false;
const context = vm.createContext({
  document: { getElementById: element },
  window: { addEventListener() {}, EventSource: true },
  EventSource: class { constructor() { stream = this; } },
  fetch: () => new Promise(() => {}),
  setInterval: () => 42,
  clearInterval: () => { cleared = true; },
});
vm.runInContext(fs.readFileSync(path.join(__dirname, '../public/app.js'), 'utf8'), context);
const run = (code) => vm.runInContext(code, context);
run(`renderMemory({ valid:true, total:8227262464, used:1717448704, available:6509813760,
  percent:20.875, cached:5091618816, buffers:804708352, swapTotal:0 });`);
assert.equal(element('memoryValue').textContent, '21%');
assert.equal(element('memoryUsedSegment').style.width, '20.875%');
assert.equal(element('memoryCacheSegment').style.width, '0%');
run('renderMemory({valid:false})');
assert.equal(element('memoryValue').textContent, '--');
assert.equal(element('memoryUsed').textContent, '--');
assert.equal(run('formatBytes(1024)'), '1.0 KiB');
assert.equal(run('formatRate(null)'), '--');
assert.equal(run('formatPercent(null)'), '--');
assert.equal(run('formatDateTime("bad")'), '--');
assert.equal(run('formatDateTime("")'), '--');
assert.notEqual(run('formatDateTime("2026-09-06T05:24:42Z")'), '--');
assert.equal(run('getChartMax([{load:175}], "percent", [{key:"load"}])'), 200);
assert.equal(run('formatPressure({some:{avg10:0}, full:{avg10:0}})'), 'some 0.00% / full 0.00%');
run('renderDisk({valid:false,io:{valid:false}})');
assert.equal(element('diskValue').textContent, '--');
assert.equal(element('diskReadRate').textContent, '--');
run('renderNetwork({valid:false}, {valid:false})');
assert.equal(element('rxRate').textContent, '--');
assert.equal(element('tcpEstablished').textContent, '--');
run(`renderCpu({cpu:{valid:false,cores:8,loadValid:false}, temperature:{max:99}})`);
assert.equal(element('cpuValue').textContent, '--');
assert.equal(element('cpuTemp').textContent, '--');
run(`renderProcessTable(els.topCpuTable, [{pid:1,name:'test',cpu:300,cpuValid:true,rss:0}], 'cpu')`);
assert.ok(element('topCpuTable').innerHTML.includes('300.0%'));
run(`renderPortListeners([{port:80,protocol:'tcp'}])`);
assert.ok(element('portListeners').innerHTML.includes('不可读取（权限或进程退出）'));
const calls = [];
context.lineContext = { beginPath() {}, stroke() {}, moveTo: () => calls.push('move'), lineTo: () => calls.push('line') };
run(`drawLine(lineContext, [{cpu:1},{cpu:null},{cpu:2},{cpu:3}], 'cpu', '#fff', {left:0,top:0},100,100,100,1)`);
assert.deepEqual(calls, ['move','move','line']);
run('startFallbackPolling()');
assert.equal(run('state.fallbackTimer'), 42);
// long: SSE 恢复时先清除轮询；这里只验证清理边界，不用伪造完整指标绕过渲染断言。
assert.throws(() => stream.onmessage({data:'invalid-json'}));
assert.ok(cleared);
assert.equal(run('state.fallbackTimer'), null);
console.log('Frontend metrics, unknown states, chart gaps and SSE recovery verified');
