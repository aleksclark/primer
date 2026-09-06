import { spawn } from 'node:child_process';
import { readFile, access } from 'node:fs/promises';
import { setTimeout as delay } from 'node:timers/promises';

const exited = child => child.exitCode !== null || child.signalCode !== null;
async function waitExit(child, milliseconds) {
  if (exited(child) || !child.pid) return;
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => { child.off('exit', done); reject(new Error(`Owned fixture PID ${child.pid} failed to terminate`)); }, milliseconds);
    function done() { clearTimeout(timer); resolve(); }
    child.once('exit', done);
    if (exited(child)) done();
  });
}

/** Owns the launcher process group; never accepts an external server or DB. */
export async function startS17Fixture(studioRoot, log = () => {}) {
  const child = spawn('./scripts/browser-fixture.sh', [], {
    cwd: studioRoot, detached: true,
    env: { ...process.env, STUDIO_TEST_DATABASE_URL: '' },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let output = '';
  const fixture = { child, url: undefined, dir: undefined };
  try {
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => finish(new Error(`Fixture readiness timed out: ${output}`)), 120_000);
      function finish(error) {
        clearTimeout(timer); child.off('error', onError); child.off('exit', onExit);
        if (error) reject(error); else resolve();
      }
      const onError = error => finish(error);
      const onExit = code => finish(new Error(`Fixture exited before readiness (${code}): ${output}`));
      function append(chunk) {
        output = (output + chunk.toString()).slice(-16_000);
        fixture.url ??= output.match(/^S17_FIXTURE_URL=(.+)$/m)?.[1];
        fixture.dir ??= output.match(/^S17_FIXTURE_DIR=(.+)$/m)?.[1];
        if (fixture.url && fixture.dir) finish();
      }
      child.stdout.on('data', append); child.stderr.on('data', append);
      child.once('error', onError); child.once('exit', onExit);
    });
    if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(fixture.url) || !/^\/tmp\/primer-s17-browser\.[\w]+$/.test(fixture.dir)) {
      throw new Error('Fixture printed an unexpected URL or owned directory');
    }
    log(`Fixture ready: launcher=${child.pid} url=${fixture.url} dir=${fixture.dir}`);
    return fixture;
  } catch (error) {
    await stopS17Fixture(fixture, log);
    throw error;
  }
}

export async function stopS17Fixture(fixture, log = () => {}) {
  if (!fixture) return;
  const { child, dir } = fixture;
  if (!exited(child) && child.pid) {
    // Once ready, let the real Go test host finish PG/JWKS cleanup before the
    // launcher's EXIT trap removes its temp directory. Verify PID ownership.
    if (dir) {
      const pid = Number((await readFile(`${dir}/host.pid`, 'utf8')).trim());
      if (!Number.isSafeInteger(pid) || pid < 2) throw new Error('Invalid owned host PID');
      const command = await readFile(`/proc/${pid}/cmdline`, 'utf8');
      if (!command.startsWith(`${dir}/fixture.test\0`)) throw new Error('Refusing to signal an unowned host');
      process.kill(pid, 'SIGTERM');
    } else {
      // Covers build/startup failure before a host.pid exists; negative pid is
      // only this detached launcher's process group, not other local services.
      process.kill(-child.pid, 'SIGTERM');
    }
    await waitExit(child, 30_000);
  }
  if (dir) {
    // The launcher, not this helper, owns directory removal.
    for (let n = 0; n < 20; n++) {
      try { await access(dir); } catch (error) { if (error.code === 'ENOENT') { log('Fixture cleanup verified (launcher exited, owned directory removed)'); return; } throw error; }
      await delay(50);
    }
    throw new Error(`Owned fixture directory remained after launcher exit: ${dir}`);
  }
  log('Fixture startup process terminated');
}
