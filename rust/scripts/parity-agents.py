#!/usr/bin/env python3
"""Differential black-box resource scenarios. No live provider credentials required."""
import argparse
import difflib
import json
import os
from pathlib import Path
import re
import subprocess
import sqlite3
import sys
import struct
import tempfile
import time
import yaml


def allowed_comparison(label, value):
    reasons = []
    if label == 'mock-requests':
        # Title generation and the model turn run concurrently on both daemons.
        value = '\n'.join(sorted(value.splitlines()))
        reasons.append('concurrent title and conversation requests may arrive in either order')
    if label == 'agent-missing-prompt-file':
        # Rust uses a stable cross-platform NotFound diagnostic instead of libc text.
        value = value.replace(': file does not exist\n', ': no such file or directory\n')
        reasons.append('Rust uses portable NotFound wording rather than Go OS-specific text')
    if label in {'agent-missing', 'model-missing', 'modelprovider-missing', 'provider-bad-type',
                 'agent-missing-model', 'agent-missing-prompt-file', 'model-missing-provider',
                 'agent-delete-missing', 'model-delete-missing', 'provider-delete-missing'}:
        # Cobra appends full usage on errors; Rust deliberately emits just the error.
        value = value.split('Usage:\n', 1)[0]
        reasons.append('Go appends usage to CLI errors; Rust keeps errors concise')
    lists = {'agent-list-json', 'agent-list-yaml', 'agent-list-table',
             'model-list-json', 'model-list-yaml', 'model-list-table',
             'model-after-delete', 'wire-agent-list', 'wire-model-list', 'turn-messages', 'wire-turn-messages'}
    if label.startswith('cross-') and label[6:] in lists:
        label = label[6:]
    if label in lists:
        # Go resource queries have no ORDER BY; Rust has deterministic time/ID order.
        reasons.append('Go list order is unspecified; Rust uses deterministic time/ID order')
        if label.endswith('-table'):
            header, *rows = value.splitlines()
            value = '\n'.join([header, *sorted(rows)])
        else:
            prefix = ''
            suffix = ''
            if value.startswith('exit='):
                prefix, value = value.split('\n', 1)
            if '\nHTTP=' in value:
                value, suffix = value.rsplit('\nHTTP=', 1)
            data = yaml.safe_load(value)
            if isinstance(data, dict):
                for key in ('agents', 'models', 'messages'):
                    if key in data:
                        data[key] = sorted(data[key], key=lambda x: json.dumps(x, sort_keys=True))
            elif isinstance(data, list):
                if label == 'turn-messages':
                    # Go's CLI renders tool-only message bodies empty; Rust exposes tool records.
                    for item in data:
                        if item.get('content', '').startswith(('[tool call]', '[tool result]')):
                            item['content'] = ''
                    reasons.append('Rust displays tool-only messages that Go CLI renders empty')
                data.sort(key=lambda x: json.dumps(x, sort_keys=True))
            value = prefix + json.dumps(data, sort_keys=True) + suffix
    return value, '; '.join(reasons)


def normalize(value, root):
    value = value.replace(str(root), '<ROOT>')
    value = re.sub(r'\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b', '<ID>', value)
    value = re.sub(r'\d{4}-\d\d-\d\d[T ][\d:.]+(?:Z|[+-]\d\d:\d\d)?', '<TIME>', value)
    prefix, suffix = '', ''
    if value.startswith('exit='):
        prefix, value = value.split('\n', 1)
        prefix += '\n'
    if '\nHTTP=' in value:
        value, suffix = value.rsplit('\nHTTP=', 1)
        suffix = '\nHTTP=' + suffix
    try:
        value = json.dumps(json.loads(value), indent=2, sort_keys=True) + '\n'
    except ValueError:
        if value.startswith('{') and len(value.splitlines()) > 1:
            value = '\n'.join(json.dumps(json.loads(line), sort_keys=True) for line in value.splitlines()) + '\n'
    return prefix + value + suffix


def stack(binary, other, root):
    root.mkdir()
    env = dict(os.environ, HOME=str(root), XDG_CONFIG_HOME=str(root/'config'),
               XDG_DATA_HOME=str(root/'data'), XDG_STATE_HOME=str(root/'state'), NO_COLOR='1')
    config = root/'config/construct'
    config.mkdir(parents=True)
    (config/'config.yaml').write_text('secret:\n  provider: file\n')
    socket = root/'daemon.sock'
    outputs = {}

    def cli(label, *args, client=binary, expected=0):
        result = subprocess.run([client, *args], env=env, text=True, capture_output=True, timeout=30)
        outputs[label] = f'exit={result.returncode}\n' + result.stdout + result.stderr
        if expected == 0 and result.returncode:
            raise RuntimeError(f'{label}: {outputs[label]}')
        if expected not in (0, None) and not result.returncode:
            raise RuntimeError(f'{label}: expected failure')
        return (result.stdout + result.stderr).strip()

    def rpc(label, resource, method, body, expected=200):
        result = subprocess.run(['curl', '-sS', '--max-time', '10', '--unix-socket', str(socket),
                                 '-H', 'Content-Type: application/json', '-d', json.dumps(body),
                                 '-w', '\nHTTP=%{http_code}\n',
                                 f'http://localhost/api/construct.v1.{resource}Service/{method}'],
                                capture_output=True, text=True, check=True)
        outputs[label] = result.stdout
        status = int(result.stdout.rsplit('HTTP=', 1)[1])
        if status != expected:
            raise RuntimeError(f'{label}: expected HTTP {expected}, got {result.stdout}')

    workspace = root/'workspace'
    workspace.mkdir()
    mock = subprocess.Popen([sys.executable, str(Path(__file__).with_name('e2e-mock.py')),
                             '--workspace', str(workspace), '--port-file', str(root/'mock.port'),
                             '--requests', str(root/'requests.jsonl')])
    for _ in range(100):
        if (root/'mock.port').exists():
            break
        time.sleep(.05)
    env['ANTHROPIC_BASE_URL'] = 'http://127.0.0.1:' + (root/'mock.port').read_text() + '/'
    with (root/'daemon.log').open('w') as log:
        daemon = subprocess.Popen([binary, 'daemon', 'run', '--listen-unix', str(socket)], env=env, stdout=log, stderr=log)
        try:
            for _ in range(200):
                if daemon.poll() is not None:
                    raise RuntimeError((root/'daemon.log').read_text())
                if socket.exists():
                    health = subprocess.run(['curl', '-sf', '--unix-socket', str(socket), 'http://localhost/healthz'], capture_output=True)
                    if health.returncode == 0:
                        break
                time.sleep(.1)
            else:
                raise RuntimeError('daemon readiness timeout')
            cli('context-add', 'context', 'add', 'local', '--endpoint', str(socket))
            cli('context-use', 'context', 'use', 'local')
            for resource in ('agent', 'model', 'modelprovider'):
                for fmt in ('table', 'json', 'yaml'):
                    cli(f'{resource}-empty-{fmt}', resource, 'list', '--output', fmt)
            provider = cli('provider-create', 'modelprovider', 'create', 'mock', '--type', 'anthropic', '--api-key', 'fake-parity-key')
            duplicate = cli('provider-duplicate', 'modelprovider', 'create', 'mock', '--type', 'anthropic', '--api-key', 'fake-parity-key', expected=None)
            cli('provider-duplicate-delete', 'modelprovider', 'delete', duplicate, '--force')
            cli('provider-bad-type', 'modelprovider', 'create', 'bad', '--type', 'bogus', '--api-key', 'fake', expected=1)
            rpc('wire-provider-bad-type', 'ModelProvider', 'CreateModelProvider', {'name': 'unsupported', 'providerType': 'MODEL_PROVIDER_TYPE_OPENAI', 'apiKey': 'fake'}, expected=400)
            model = cli('model-create', 'model', 'create', 'parity-model', '--provider', provider, '--context-window', '128000')
            edit_model = cli('edit-model-create', 'model', 'create', 'edit-model', '--provider', provider, '--context-window', '64000')
            env['PARITY_EDIT_MODEL'] = edit_model
            agent = cli('agent-create', 'agent', 'create', 'parity-agent', '--model', model, '--prompt', 'Parity prompt', '--description', 'Parity description')
            prompt = root/'prompt.txt'
            prompt.write_text('Prompt from file\n')
            file_agent = cli('agent-file', 'agent', 'create', 'file-agent', '--model', model, '--prompt-file', str(prompt))
            cli('agent-missing-model', 'agent', 'create', 'bad', '--model', 'absent-parity', '--prompt', 'prompt', expected=1)
            cli('agent-missing-prompt-file', 'agent', 'create', 'bad', '--model', model, '--prompt-file', str(root/'absent-prompt'), expected=1)
            cli('model-missing-provider', 'model', 'create', 'bad', '--provider', 'absent-parity', '--context-window', '128000', expected=1)
            for fmt in ('table', 'json', 'yaml'):
                cli(f'agent-builtin-{fmt}', 'agent', 'get', 'edit', '--output', fmt)
                cli(f'model-builtin-{fmt}', 'model', 'get', 'claude-sonnet-4-5-20250929', '--output', fmt)
            for resource, ident, wire, plural in [('modelprovider', provider, 'ModelProvider', 'ModelProviders'), ('model', model, 'Model', 'Models'), ('agent', agent, 'Agent', 'Agents')]:
                for fmt in ('table', 'json', 'yaml'):
                    cli(f'{resource}-list-{fmt}', resource, 'list', '--output', fmt)
                    cli(f'{resource}-get-{fmt}', resource, 'get', ident, '--output', fmt)
                    cli(f'cross-{resource}-{fmt}', resource, 'get', ident, '--output', fmt, client=other)
                    cli(f'cross-{resource}-list-{fmt}', resource, 'list', '--output', fmt, client=other)
                cli(f'{resource}-missing', resource, 'get', 'absent-parity', expected=1)
                rpc(f'wire-{resource}-list', wire, 'List'+plural, {})
                rpc(f'wire-{resource}-get', wire, 'Get'+wire, {'id': ident})
                rpc(f'wire-{resource}-missing', wire, 'Get'+wire, {'id': '11111111-1111-4111-8111-111111111111'}, expected=404)
                rpc(f'wire-{resource}-invalid', wire, 'Get'+wire, {'id': 'bad'}, expected=400)
            rpc('wire-model-update', 'Model', 'UpdateModel', {'id': model, 'contextWindow': '64000', 'alias': 'parity-alias'})
            cli('model-updated', 'model', 'get', model, '--output', 'json')
            cli('agent-edit', 'agent', 'edit', agent, client=binary)
            cli('agent-edited', 'agent', 'get', agent, '--output', 'json')
            # Run the turn with the custom model/prompt, independently of editor coverage.
            rpc('wire-agent-restore', 'Agent', 'UpdateAgent', {'id': agent, 'modelId': model, 'instructions': 'Parity prompt'})
            # The Go CLI cannot set URLs; Rust deliberately uses the stored URL.
            with sqlite3.connect(root/'data/construct/construct.db') as db:
                db.execute('UPDATE model_providers SET url = ? WHERE id = ?', (env['ANTHROPIC_BASE_URL'].rstrip('/'), provider))
            task = cli('task-create', 'task', 'create', '--agent', agent, '--workspace', str(workspace))
            subscription = json.dumps({'taskId': task}).encode()
            events_file = (root/'events.bin').open('wb')
            events = subprocess.Popen(['curl', '-sS', '--max-time', '4', '--unix-socket', str(socket),
                                       '-H', 'Content-Type: application/connect+json',
                                       '-H', 'Connect-Protocol-Version: 1', '--data-binary', '@-',
                                       'http://localhost/api/construct.v1.EventService/Subscribe'],
                                      stdin=subprocess.PIPE, stdout=events_file, stderr=subprocess.DEVNULL)
            events.stdin.write(b'\0' + struct.pack('>I', len(subscription)) + subscription)
            events.stdin.close()
            time.sleep(.2)
            cli('message-create', 'message', 'create', task, 'GO_INITIAL')
            for _ in range(200):
                messages = cli('turn-messages', 'message', 'list', '--task', task, '--output', 'json', '--wide')
                if 'MOCK_REPLY GO_INITIAL tool-ok' in messages:
                    break
                time.sleep(.1)
            else:
                raise RuntimeError('model turn timed out')
            outputs['mock-requests'] = (root/'requests.jsonl').read_text()
            rpc('wire-turn-messages', 'Message', 'ListMessages', {'filter': {'taskIds': task}})
            events.wait(timeout=10)
            events_file.close()
            data = (root/'events.bin').read_bytes()
            decoded = []
            while data:
                flags, length = data[0], struct.unpack('>I', data[1:5])[0]
                decoded.append({'flags': flags, 'body': json.loads(data[5:5+length])})
                data = data[5+length:]
            if not decoded:
                raise RuntimeError('event subscription produced no events')
            outputs['turn-events'] = json.dumps(decoded)
            cli('agent-file-delete', 'agent', 'delete', file_agent, '--force')
            cli('agent-delete', 'agent', 'delete', agent, '--force')
            cli('model-delete', 'model', 'delete', model, '--force')
            cli('edit-model-delete', 'model', 'delete', edit_model, '--force')
            cli('provider-delete', 'modelprovider', 'delete', provider, '--force')
            for resource, label in [('agent', 'agent'), ('model', 'model'), ('modelprovider', 'provider')]:
                cli(f'{label}-delete-missing', resource, 'delete', 'absent-parity', '--force', expected=1)
            for resource in ('agent', 'model', 'modelprovider'):
                cli(f'{resource}-after-delete', resource, 'list', '--output', 'json', expected=None)
        finally:
            daemon.terminate()
            daemon.wait(timeout=20)
            mock.terminate()
            mock.wait(timeout=10)
            for label, value in outputs.items():
                (root/(label+'.txt')).write_text(normalize(value, root))
    return outputs.keys()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for option in ('go', 'rust', 'output'):
        parser.add_argument('--'+option, required=True)
    args = parser.parse_args()
    root = Path(tempfile.mkdtemp(prefix='agents-', dir=args.output))
    print(f'Artifacts: {root}', flush=True)
    os.environ['EDITOR'] = str(Path(__file__).with_name('parity-agent-editor.py'))
    labels = stack(args.go, args.rust, root/'go')
    stack(args.rust, args.go, root/'rust')
    failures = allowed = 0
    for label in sorted(labels):
        left = (root/'go'/(label+'.txt')).read_text()
        right = (root/'rust'/(label+'.txt')).read_text()
        if left != right:
            comparable_left, reason = allowed_comparison(label, left)
            comparable_right, _ = allowed_comparison(label, right)
            if reason and comparable_left == comparable_right:
                allowed += 1
                print(f'ALLOW: {label}: {reason}')
                continue
            failures += 1
            print('DIFF:', label)
            (root/(label+'.diff')).write_text(''.join(difflib.unified_diff(left.splitlines(True), right.splitlines(True), fromfile='go', tofile='rust')))
    print(f'Parity: {len(labels)-failures-allowed} matched, {allowed} allowlisted, {failures} unexpected differences')
    raise SystemExit(bool(failures))


if __name__ == '__main__':
    main()
