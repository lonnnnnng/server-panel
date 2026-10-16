const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const source = fs.readFileSync(path.join(__dirname, 'service.sh'), 'utf8');
const health = source.match(/^health\(\) \{[\s\S]*?^\}/m);
assert.ok(health, 'health function must exist');

// long: 等待就绪会在 if 中调用 health，必须覆盖 Bash 在此场景不触发 errexit 的失败路径。
for (const [mock, expected] of [['return 7', 'unhealthy'], ['return 0', 'healthy']]) {
  const script = `set -euo pipefail
PANEL_CONFIG=/dev/null
${health[0]}
curl() { ${mock}; }
if health >/dev/null 2>&1; then printf healthy; else printf unhealthy; fi
`;
  const result = spawnSync('bash', ['-c', script], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, expected, `health must propagate curl result: ${mock}`);
}
console.log('Service health success/failure propagation verified');
