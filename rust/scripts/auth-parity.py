#!/usr/bin/env python3
"""Linux, stdlib-only differential auth oracle. Never prints bearer credentials."""
import base64
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import signal
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time


class UnixHTTP(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(5)
        self.sock.connect(self.host)


class Daemon:
    def __init__(self, binary, root, tcp=False):
        self.root = root
        self.env = dict(os.environ, HOME=str(root))
        for name in ('CONFIG', 'DATA', 'CACHE', 'STATE', 'RUNTIME'):
            self.env[f'XDG_{name}_HOME' if name != 'RUNTIME' else 'XDG_RUNTIME_DIR'] = str(root / name.lower())
        config = root / 'config/construct'
        config.mkdir(parents=True, exist_ok=True)
        (config / 'config.yaml').write_text('secret:\n  provider: file\n')
        self.address = str(root / 'api.sock')
        args = [binary, 'daemon', 'run', '--listen-unix', self.address]
        listener = None
        kwargs = {}
        if tcp:
            listener = socket.socket()
            listener.bind(('127.0.0.1', 0))
            listener.listen()
            self.address = listener.getsockname()
            fd = listener.fileno()
            kwargs = dict(pass_fds=(fd,))
            self.env['LISTEN_FDS'] = '1'
            args = ['/bin/bash', '-c', 'eval "exec 3<&$1"; shift; export LISTEN_PID=$$; exec "$@"', 'activation', str(fd), binary, 'daemon', 'run']
        self.tcp = tcp
        self.log = open(root / ('tcp.log' if tcp else 'unix.log'), 'w')
        self.process = subprocess.Popen(args, env=self.env, stdout=self.log, stderr=self.log, **kwargs)
        if listener:
            listener.close()
        for _ in range(300):
            assert self.process.poll() is None, f'daemon exited; inspect {self.log.name}'
            try:
                self.rpc('ListTokens')
                break
            except (OSError, http.client.HTTPException):
                time.sleep(.05)
        else:
            raise AssertionError(f'daemon readiness timeout: {self.log.name}')

    def rpc(self, method, body=None, authorization=None):
        connection = http.client.HTTPConnection(*self.address, timeout=5) if self.tcp else UnixHTTP(self.address, timeout=5)
        headers = {'Content-Type': 'application/json', 'Connect-Protocol-Version': '1', 'Host': 'localhost'}
        if authorization is not None:
            headers['Authorization'] = authorization
        try:
            service = 'AgentService' if method == 'ListAgents' else 'AuthService'
            connection.request('POST', f'/api/construct.v1.{service}/' + method, json.dumps(body or {}), headers)
            response = connection.getresponse()
            payload = response.read()
            return response.status, json.loads(payload or '{}')
        finally:
            connection.close()

    def stop(self):
        self.process.send_signal(signal.SIGTERM)
        self.process.wait(timeout=20)
        self.log.close()


def exercise(binary, root):
    transcript = []
    daemon = Daemon(binary, root)
    tokens = {}
    def call(method, body=None, authorization=None, success=None):
        status, value = daemon.rpc(method, body, authorization)
        if success is not None:
            assert (status == 200) == success, (method, status, value if status != 200 else 'unexpected success')
        if status != 200:
            transcript.append((method, status, value))
        return value
    try:
        assert call('ListTokens', success=True).get('tokens', []) == []
        for method, body in [
            ('CreateToken', {}), ('CreateToken', {'name': 'too-long', 'expiresIn': '31536001s'}),
            ('CreateSetupCode', {}), ('CreateSetupCode', {'tokenName': 'x', 'expiresIn': '259201s'}),
            ('CreateSetupCode', {'tokenName': 'x', 'tokenExpiresIn': '31536001s'}),
            ('ExchangeSetupCode', {}), ('ExchangeSetupCode', {'setupCode': 'INVALID'}),
            ('RevokeToken', {'id': 'bad'}), ('RevokeToken', {'id': '00000000-0000-0000-0000-000000000001'}),
        ]:
            call(method, body, success=False)
        for name, expiry in [('active', '3600s'), ('expired', '-1s'), ('revoked', '3600s')]:
            tokens[name] = call('CreateToken', {'name': name, 'expiresIn': expiry}, success=True)['token']
            assert re.fullmatch(r'ct_[A-Za-z0-9_-]{43}', tokens[name])
            assert len(base64.urlsafe_b64decode(tokens[name][3:] + '=')) == 32
        call('CreateToken', {'name': 'active'}, success=False)
        listing = call('ListTokens', {'includeExpired': True}, success=True)['tokens']
        transcript.append(('list', [(t['name'], t.get('isActive', False)) for t in listing]))
        assert [t['name'] for t in call('ListTokens', {'namePrefix': 'act'}, success=True)['tokens']] == ['active']
        assert 'expired' not in [t['name'] for t in call('ListTokens', success=True)['tokens']]
        revoked = next(t['id'] for t in listing if t['name'] == 'revoked')
        call('RevokeToken', {'id': revoked}, success=True)
        call('RevokeToken', {'id': revoked}, success=False)
        code = call('CreateSetupCode', {'tokenName': 'enrolled'}, success=True)['setupCode']
        assert re.fullmatch(r'[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}', code)
        tokens['enrolled'] = call('ExchangeSetupCode', {'setupCode': code.lower()}, success=True)['token']
        call('ExchangeSetupCode', {'setupCode': code}, success=False)
        for name, expiry in [('expired-code', '-1s'), ('active', '60s')]:
            code = call('CreateSetupCode', {'tokenName': name, 'expiresIn': expiry}, success=True)['setupCode']
            call('ExchangeSetupCode', {'setupCode': code}, success=False)
            call('ExchangeSetupCode', {'setupCode': code}, success=False)
        # Unix administration ignores even malformed Authorization headers.
        call('ListTokens', authorization='Basic nonsense', success=True)
    finally:
        daemon.stop()
    database = root / 'data/construct/construct.db'
    with sqlite3.connect(database) as db:
        stored = dict(db.execute('select name, token_hash from tokens'))
        for name in ('active', 'expired', 'enrolled'):
            assert stored[name] == hashlib.sha256(tokens[name].encode()).digest()
    daemon = Daemon(binary, root, tcp=True)
    try:
        for header in [None, 'Bearer', 'bearer ct_x', 'Basic ct_x', 'Bearer abc', 'Bearer ct_x', 'Bearer  ct_x']:
            call('ListTokens', authorization=header, success=False)
        for name, token in tokens.items():
            agents = call('ListAgents', authorization='Bearer ' + token, success=name in ('active', 'enrolled'))
            if name in ('active', 'enrolled'):
                assert agents.get('agents', []) == []
            for method in ('ListTokens', 'CreateToken', 'CreateSetupCode', 'RevokeToken'):
                call(method, authorization='Bearer ' + token, success=False)
        call('ExchangeSetupCode', {'setupCode': 'INVALID'}, success=False)
    finally:
        daemon.stop()
    return transcript


def contexts(binary, root):
    root.mkdir()
    env = dict(os.environ, HOME=str(root), XDG_CONFIG_HOME=str(root / 'config'), XDG_DATA_HOME=str(root / 'data'))
    result = []
    for args in [
        ['current'], ['list', '-o', 'json'], ['use', '-'], ['use', 'missing'],
        ['add', 'local', '--endpoint', '/test/socket'], ['current'],
        ['add', 'remote', '--endpoint', 'http://localhost:1234', '--set-current'],
        ['list', '-o', 'json'], ['use', 'local'], ['use', '-'],
        ['remove', 'remote', '--force'], ['current'], ['remove', 'missing', '--force'],
        ['add', 'invalid', '--endpoint', '/socket', '--kind', 'tcp'],
        ['remove', 'local', '--force'], ['list', '-o', 'json'],
    ]:
        output = subprocess.run([binary, 'context', *args], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)
        path = root / 'config/construct/context.yaml'
        text = output.stdout.decode()
        if output.returncode:
            # Cobra appends usage on runtime errors; clap only prints the error.
            # Compare the diagnostic and exit code, not framework-specific help.
            first, _, rest = text.partition('\n')
            assert not rest or rest.startswith('Usage:'), text
            text = first + '\n'
        result.append((args, output.returncode, text, path.read_text() if path.exists() else None))
        if path.exists():
            assert path.stat().st_mode & 0o777 == 0o600
    return result


def main():
    go, rust = map(os.path.abspath, sys.argv[1:])
    root = Path(tempfile.mkdtemp(prefix='run-', dir=Path(__file__).resolve().parents[1] / 'target/auth-parity'))
    print(f'Auth parity artifacts: {root}', flush=True)
    print('NOTE context runtime-error usage trailers differ (Cobra vs clap); diagnostics and exit codes are compared', flush=True)
    for label, function in [('RPC', exercise), ('context', contexts)]:
        expected = function(go, root / (label + '-go'))
        actual = function(rust, root / (label + '-rust'))
        (root / (label + '-go.json')).write_text(json.dumps(expected, indent=2))
        (root / (label + '-rust.json')).write_text(json.dumps(actual, indent=2))
        if actual != expected:
            import difflib
            print('\n'.join(difflib.unified_diff(json.dumps(expected, indent=2).splitlines(), json.dumps(actual, indent=2).splitlines(), fromfile='Go', tofile='Rust')))
            raise AssertionError(label + ' differs')
        print(f'PASS {label}: Go and Rust outputs, errors, statuses and persisted state match', flush=True)


if __name__ == '__main__':
    main()
