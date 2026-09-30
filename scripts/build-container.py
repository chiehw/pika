#!/usr/bin/env python3
"""Build the Linux servers, agents and frontend resources required by Dockerfile."""
import argparse
import concurrent.futures
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parent.parent
AGENTS = [('linux', 'amd64', ''), ('linux', 'arm64', ''), ('linux', 'arm', '7'), ('linux', 'loong64', ''), ('darwin', 'amd64', ''), ('darwin', 'arm64', ''), ('windows', 'amd64', ''), ('windows', 'arm64', '')]


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
        raise SystemExit('The pinned default theme checkout is required')
    if not args.skip_web:
        for directory in [ROOT / 'web', theme]:
            run(['npm', 'ci', '--prefix', str(directory), '--registry=https://registry.npmjs.org'])
            run(['npm', 'run', 'build', '--prefix', str(directory)])
    for directory in [ROOT / 'web/dist', theme / 'dist']:
        if not (directory / 'index.html').is_file():
            raise SystemExit(f'Missing frontend build: {directory}')
    output_theme = ROOT / 'themes/default'
    if output_theme.exists():
        shutil.rmtree(output_theme)
    shutil.copytree(theme / 'dist', output_theme / 'dist')
    shutil.copy2(theme / 'pika-theme.json', output_theme / 'pika-theme.json')
    agents = ROOT / 'bin/agents'
    if agents.exists():
        shutil.rmtree(agents)
    agents.mkdir(parents=True)
    agent_version = subprocess.check_output(['git', 'log', '-1', '--format=%h', '--', 'pkg/agent'], cwd=ROOT, text=True).strip()
    ldflags = f'-s -w -X github.com/pika-monitor/pika/pkg/version.Version={args.version} -X github.com/pika-monitor/pika/pkg/version.AgentVersion={agent_version}'
    jobs = []
    for goos, arch, goarm in AGENTS:
        label = 'armv7' if goarm else arch
        suffix = '.exe' if goos == 'windows' else ''
        jobs.append((goos, arch, goarm, './cmd/agent', agents / f'pika-agent-{goos}-{label}{suffix}'))
    for arch in ['amd64', 'arm64']:
        jobs.append(('linux', arch, '', './cmd/serv', ROOT / 'bin' / f'pika-linux-{arch}'))

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
    print('Container build context ready: 2 Linux servers, 8 agents, admin frontend and default theme', flush=True)


if __name__ == '__main__':
    main()
