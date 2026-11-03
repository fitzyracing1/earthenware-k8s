#!/usr/bin/env python3
"""Earthenware node agent: writes MEASURED readings as node annotations.

- earthenware.io/temp-c : max of /sys/class/thermal/thermal_zone*/temp (or heatgate if installed).
  If no sensor exists (common on VMs) the annotation is removed/not written, and we log that.
- earthenware.io/cpu-pct : CPU busy % measured from /proc/stat over INTERVAL seconds.
Stdlib only; talks to the API server with the in-cluster service account.
"""
import glob, json, os, ssl, sys, time, urllib.request

NODE = os.environ["NODE_NAME"]
INTERVAL = float(os.environ.get("INTERVAL", "15"))
SA = "/var/run/secrets/kubernetes.io/serviceaccount"
HOST = os.environ.get("KUBERNETES_SERVICE_HOST"); PORT = os.environ.get("KUBERNETES_SERVICE_PORT", "443")


def read_temp_c():
    try:  # optional heatgate
        import heatgate  # noqa
        for fn in ("read_temp_c", "max_temp_c"):
            f = getattr(heatgate, fn, None)
            if f:
                v = f()
                if v is not None:
                    return float(v)
    except Exception:
        pass
    vals = []
    for p in glob.glob("/sys/class/thermal/thermal_zone*/temp"):
        try:
            v = int(open(p).read().strip()) / 1000.0
            if v > 0:
                vals.append(v)
        except Exception:
            pass
    return max(vals) if vals else None


def cpu_times():
    f = [int(x) for x in open("/proc/stat").readline().split()[1:]]
    idle = f[3] + (f[4] if len(f) > 4 else 0)
    return sum(f), idle


def patch(ann):
    token = open(f"{SA}/token").read()
    ctx = ssl.create_default_context(cafile=f"{SA}/ca.crt")
    body = json.dumps({"metadata": {"annotations": ann}}).encode()
    req = urllib.request.Request(f"https://{HOST}:{PORT}/api/v1/nodes/{NODE}", data=body, method="PATCH",
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/merge-patch+json"})
    urllib.request.urlopen(req, context=ctx, timeout=10).read()


def main():
    t0, i0 = cpu_times()
    warned = False
    while True:
        time.sleep(INTERVAL)
        t1, i1 = cpu_times()
        cpu = 100.0 * (1 - (i1 - i0) / max(t1 - t0, 1)); t0, i0 = t1, i1
        temp = read_temp_c()
        ann = {"earthenware.io/cpu-pct": f"{cpu:.1f}"}
        if temp is None:
            ann["earthenware.io/temp-c"] = None  # merge-patch null => remove; never guess
            if not warned:
                print(f"node {NODE}: no thermal sensor found; not writing temperature", flush=True); warned = True
        else:
            ann["earthenware.io/temp-c"] = f"{temp:.1f}"
        try:
            patch(ann)
        except Exception as e:
            print(f"patch failed: {e}", file=sys.stderr, flush=True)


if __name__ == "__main__":
    main()
