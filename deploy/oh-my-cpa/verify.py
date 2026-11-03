#!/usr/bin/env python3
"""Collect a small, redacted, read-only CPA baseline."""
import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

PATHS = ("server/info", "config/config-version", "credentials",
         "routing/codex-account", "observability/requests/recent")
PROPERTIES = ("MainPID", "ExecMainStartTimestampMonotonic", "NRestarts",
              "ActiveState", "SubState")


class VerificationError(Exception):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def root_url(value):
    parsed = urllib.parse.urlsplit(value)
    if (parsed.scheme != "http" or parsed.hostname != "127.0.0.1" or
            parsed.username is not None or parsed.password is not None or
            parsed.path not in ("", "/") or parsed.query or parsed.fragment):
        raise VerificationError("proxy root must be canonical http://127.0.0.1[:port]")
    try:
        port = parsed.port
    except ValueError:
        raise VerificationError("invalid proxy port") from None
    if parsed.netloc != "127.0.0.1" + (":" + str(port) if port else ""):
        raise VerificationError("proxy root must be canonical http://127.0.0.1[:port]")
    return value.rstrip("/")


def fetch(root, key, path, opener):
    if path not in PATHS:
        raise VerificationError("path is not allowed")
    request = urllib.request.Request(root + "/v8/management/" + path,
                                     headers={"Authorization": "Bearer " + key})
    try:
        with opener.open(request) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        exc.close()
        if exc.code in (401, 403):
            raise VerificationError("authentication rejected; stopped") from None
        if 300 <= exc.code < 400:
            raise VerificationError("redirect refused") from None
        raise VerificationError("proxy request failed (HTTP %d)" % exc.code) from None
    except (OSError, ValueError):
        raise VerificationError("proxy request failed or returned invalid JSON") from None


def digest_file(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def system_health():
    command = ["systemctl", "--user", "show", "cliproxyapi.service"]
    for prop in PROPERTIES:
        command += ["--property", prop]
    try:
        result = subprocess.run(command, capture_output=True, text=True, check=True)
        values = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
        if any(not re.fullmatch(r"[0-9]+", values.get(p, "")) for p in PROPERTIES[:3]):
            raise ValueError()
        if any(not re.fullmatch(r"[a-z-]+", values.get(p, "")) for p in PROPERTIES[3:]):
            raise ValueError()
        return {p: values[p] for p in PROPERTIES}
    except (OSError, subprocess.SubprocessError, ValueError):
        raise VerificationError("proxy service health unavailable") from None


def collect(root, key, config_file, opener=None):
    root = root_url(root)
    opener = opener or urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    service = system_health()
    pid = int(service["MainPID"])
    if pid <= 0 or service["ActiveState"] != "active" or service["SubState"] != "running":
        raise VerificationError("proxy service is not active/running")
    try:
        stat = os.stat(config_file)
        config = {"sha256": digest_file(config_file), "size": stat.st_size,
                  "mtime_ns": stat.st_mtime_ns, "mode": stat.st_mode & 0o777}
        executable = digest_file("/proc/%d/exe" % pid)
    except OSError:
        raise VerificationError("config or runtime executable fingerprint unavailable") from None
    data = {path: fetch(root, key, path, opener) for path in PATHS}
    version = data["config/config-version"]
    routing = data["routing/codex-account"]
    credentials = data["credentials"]
    recent = data["observability/requests/recent"]
    if (type(version) is not int or version != 8 or not isinstance(routing, dict) or
            not isinstance(credentials, dict) or not isinstance(credentials.get("files"), list) or
            not isinstance(recent, dict) or not isinstance(recent.get("requests"), list)):
        raise VerificationError("unexpected proxy response shape")
    mode, strategy = routing.get("mode"), routing.get("strategy")
    auth_id = routing.get("auth_id")
    if (mode not in ("automatic", "manual") or strategy not in ("", "soonest-reset") or
            (auth_id is not None and not isinstance(auth_id, str)) or
            routing.get("applied") is not True or routing.get("home_enabled") is not False or
            (mode == "manual" and not auth_id) or (mode == "automatic" and auth_id)):
        raise VerificationError("unexpected routing response shape")
    return {"service": service, "executable_sha256": executable, "config": config,
            "config_version": version,
            "routing": {"mode": mode, "strategy": strategy,
                        "auth_id_sha256": hashlib.sha256(auth_id.encode()).hexdigest() if auth_id else None,
                        "applied": routing["applied"], "home_enabled": routing["home_enabled"]},
            "credential_count": len(credentials["files"]),
            "recent_request_count": len(recent["requests"])}


def compare(before, after):
    fields = ("service", "executable_sha256", "config", "config_version", "routing", "credential_count")
    return [field for field in fields if field not in before or field not in after or before[field] != after[field]]


def write_snapshot(path, report):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(report, stream, indent=2)
        stream.write("\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--proxy-root", required=True)
    parser.add_argument("--key-file", required=True)
    parser.add_argument("--config-file", required=True)
    parser.add_argument("--output")
    parser.add_argument("--compare")
    args = parser.parse_args()
    try:
        with open(args.key_file, encoding="utf-8") as stream:
            key = stream.read().strip()
        if not key or "\n" in key or "\r" in key:
            raise VerificationError("invalid key file")
        report = collect(args.proxy_root, key, args.config_file)
        if args.compare:
            with open(args.compare, encoding="utf-8") as stream:
                baseline = json.load(stream)
            if not isinstance(baseline, dict):
                raise VerificationError("invalid baseline")
            differences = compare(baseline, report)
        else:
            differences = []
        if args.output:
            write_snapshot(args.output, report)
        print(json.dumps({"snapshot": report, "changed": differences}, indent=2))
        return 1 if differences else 0
    except (VerificationError, OSError, ValueError):
        exc = sys.exc_info()[1]
        print(str(exc) if isinstance(exc, VerificationError) else "local file operation failed", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
