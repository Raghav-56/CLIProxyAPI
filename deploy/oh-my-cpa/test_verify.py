import contextlib
import io
import json
import os
import tempfile
import unittest
import urllib.error
from unittest import mock

import verify


class Response:
    def __init__(self, data):
        self.data = data

    def __enter__(self):
        return io.BytesIO(json.dumps(self.data).encode())

    def __exit__(self, *args):
        pass


class Opener:
    def __init__(self, data):
        self.data = data
        self.paths = []

    def open(self, request):
        path = request.full_url.split('/v8/management/')[1]
        self.paths.append(path)
        return Response(self.data[path])


class VerifyTests(unittest.TestCase):
    def setUp(self):
        self.data = {'server/info': {'hostname': 'secret-host'},
                     'config/config-version': 8,
                     'credentials': {'files': [{'id': 'secret-account'}]},
                     'routing/codex-account': {'mode': 'manual', 'strategy': '', 'auth_id': 'secret-account', 'applied': True, 'home_enabled': False},
                     'observability/requests/recent': {'requests': [{'key': 'secret-key'}]}}
        self.health = {'MainPID': '123', 'ExecMainStartTimestampMonotonic': '5', 'NRestarts': '0', 'ActiveState': 'active', 'SubState': 'running'}

    def collect(self):
        with tempfile.NamedTemporaryFile() as config, mock.patch.object(verify, 'system_health', return_value=self.health), mock.patch.object(verify, 'digest_file', return_value='hash'):
            return verify.collect('http://127.0.0.1:8317', 'secret-key', config.name, Opener(self.data))

    def test_redaction_and_scalar_version(self):
        result = self.collect()
        encoded = json.dumps(result)
        for secret in ('secret-account', 'secret-key', 'secret-host'):
            self.assertNotIn(secret, encoded)
        self.assertEqual(result['config_version'], 8)
        self.assertEqual(result['routing']['auth_id_sha256'], __import__('hashlib').sha256(b'secret-account').hexdigest())
        self.data['config/config-version'] = {'config-version': 8}
        with self.assertRaises(verify.VerificationError):
            self.collect()

    def test_error_body_and_redirect_are_not_exposed(self):
        for status in (302, 401, 403, 500):
            opener = mock.Mock()
            opener.open.side_effect = urllib.error.HTTPError('http://127.0.0.1', status, 'secret-key', {}, io.BytesIO(b'secret-account'))
            with self.assertRaises(verify.VerificationError) as caught:
                verify.fetch('http://127.0.0.1', 'secret-key', verify.PATHS[0], opener)
            self.assertNotIn('secret', str(caught.exception))
            self.assertEqual(opener.open.call_count, 1)
        self.assertIsNone(verify.NoRedirect().redirect_request(None, None, 302, '', {}, 'http://elsewhere'))

    def test_comparison(self):
        before = self.collect()
        after = json.loads(json.dumps(before))
        after['recent_request_count'] += 1
        self.assertEqual(verify.compare(before, after), [])
        after['routing']['applied'] = False
        after['config']['sha256'] = 'changed'
        after['credential_count'] += 1
        self.assertEqual(verify.compare(before, after), ['config', 'routing', 'credential_count'])

    def test_unhealthy_or_inconsistent_baselines_are_rejected(self):
        self.health['SubState'] = 'start-post'
        with self.assertRaises(verify.VerificationError):
            self.collect()
        self.health['SubState'] = 'running'
        routing = self.data['routing/codex-account']
        for patch in ({'applied': False}, {'home_enabled': True}, {'auth_id': ''},
                      {'mode': 'automatic'}):
            original = routing.copy()
            routing.update(patch)
            with self.assertRaises(verify.VerificationError):
                self.collect()
            routing.clear()
            routing.update(original)

    def test_root_and_private_output(self):
        for root in ('http://localhost:8317', 'http://127.0.0.1:8317/path', 'http://127.0.0.1@evil:8317'):
            with self.assertRaises(verify.VerificationError):
                verify.root_url(root)
        with tempfile.TemporaryDirectory() as directory:
            path = os.path.join(directory, 'snapshot')
            verify.write_snapshot(path, self.collect())
            self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                verify.write_snapshot(path, {})


if __name__ == '__main__':
    unittest.main()
