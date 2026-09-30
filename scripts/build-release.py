#!/usr/bin/env python3
"""Build Pika binaries and complete server archives without external compression tools."""
import argparse
import concurrent.futures
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parent.parent
AGENTS = [('linux', 'amd64', ''), ('linux', 'arm64', ''), ('linux', 'arm', '7'), ('linux', 'loong64', ''), ('darwin', 'amd64', ''), ('darwin', 'arm64', ''), ('windows', 'amd64', ''), ('windows', 'arm64', '')]
SERVERS = [('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'amd64'), ('darwin', 'arm64')]


def run(args, **kwargs):
    subprocess.run(args, cwd=ROOT, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--version', required=True)
    parser.add_argument('--theme-dir', default='../pika-default-theme')
    parser.add_argument('--skip-web', action='store_true')
    args = parser.parse_args()
    theme = (ROOT / args.theme_dir).resolve()
    if not (theme / 'pika-theme.json').is_file():
        raise SystemExit('Default theme checkout is required; use the commit in .github/default-theme.ref')
    if not args.skip_web:
        for directory in [ROOT / 'web', theme]:
            run(['npm', 'ci', '--prefix', str(directory), '--registry=https://registry.npmjs.org'])
            run(['npm', 'run', 'build', '--prefix', str(directory)])
    for directory in [ROOT / 'web/dist', theme / 'dist']:
        if not (directory / 'index.html').is_file():
            raise SystemExit(f'Frontend build is missing: {directory}')
    dist = ROOT / 'dist'
    if dist.exists():
        shutil.rmtree(dist)
    dist.mkdir()
    agent_version = subprocess.check_output(['git', 'log', '-1', '--format=%h', '--', 'pkg/agent'], cwd=ROOT, text=True).strip() or args.version
    ldflags = f'-s -w -X github.com/pika-monitor/pika/pkg/version.Version={args.version} -X github.com/pika-monitor/pika/pkg/version.AgentVersion={agent_version}'
    jobs = []
    for goos, arch, goarm in AGENTS:
        display_arch = 'armv7' if goarm else arch
        suffix = '.exe' if goos == 'windows' else ''
        jobs.append((goos, arch, goarm, './cmd/agent', dist / f'pika-agent-{goos}-{display_arch}{suffix}'))
    for goos, arch in SERVERS:
        jobs.append((goos, arch, '', './cmd/serv', dist / f'pika-{goos}-{arch}'))

    def build(job):
        goos, arch, goarm, package, target = job
        env = dict(os.environ, CGO_ENABLED='0', GOOS=goos, GOARCH=arch)
        env.pop('GOARM', None)
        if goarm:
            env['GOARM'] = goarm
        print(f'Building {target.name}', flush=True)
        run(['go', 'build', '-trimpath', '-ldflags', ldflags, '-o', str(target), package], env=env)
        print(f'Built {target.name}', flush=True)

    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as executor:
        list(executor.map(build, jobs))
    for goos, arch in SERVERS:
        package_name = f'pika-{args.version}-{goos}-{arch}'
        with tempfile.TemporaryDirectory(prefix='pika-package-') as temporary:
            package = Path(temporary) / package_name
            package.mkdir()
            (package / 'data').mkdir()
            (package / 'logs').mkdir()
            shutil.copy2(dist / f'pika-{goos}-{arch}', package / 'pika')
            (package / 'pika').chmod(0o755)
            shutil.copytree(ROOT / 'web/dist', package / 'web/dist')
            shutil.copytree(theme / 'dist', package / 'themes/default/dist')
            shutil.copy2(theme / 'pika-theme.json', package / 'themes/default/pika-theme.json')
            agents = package / 'bin/agents'
            agents.mkdir(parents=True)
            for job in jobs:
                if job[3] == './cmd/agent':
                    shutil.copy2(job[4], agents / job[4].name)
            for name in ['config.sqlite.yaml', 'config.postgresql.yaml', 'README.md', 'README.zh-CN.md', 'LICENSE']:
                shutil.copy2(ROOT / name, package / name)
            (package / 'docs').mkdir()
            shutil.copy2(ROOT / 'docs/log-monitor.md', package / 'docs/log-monitor.md')
            with tarfile.open(dist / f'{package_name}.tar.gz', 'w:gz') as archive:
                archive.add(package, arcname=package_name)
        print(f'Packaged {package_name}', flush=True)
    files = sorted(path for path in dist.iterdir() if path.is_file() and path.name != 'SHA256SUMS')
    with (dist / 'SHA256SUMS').open('w') as checksums:
        for path in files:
            with path.open('rb') as source:
                digest = hashlib.file_digest(source, 'sha256').hexdigest()
            checksums.write(f'{digest}  {path.name}\n')
    print(f'Release {args.version}: {len(files)} assets plus SHA256SUMS', flush=True)


if __name__ == '__main__':
    main()
