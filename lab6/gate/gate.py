#!/usr/bin/env python3
"""Скрипт-гейт канареечного релиза.

Раз в INTERVAL секунд читает из Prometheus нагрузку, долю ошибок и задержку v2
и меняет веса v1/v2 в HTTPRoute. Запросы приходят через env, поэтому гейт не
привязан к метрикам конкретного mesh.

Рестарт пода — это новый релиз: гейт снова выставляет START_WEIGHT.
"""
import http.server
import json
import math
import os
import ssl
import threading
import time
import urllib.parse
import urllib.request

PROM_URL = os.getenv("PROM_URL", "http://prometheus-operated.monitoring:9090")
RPS_QUERY = os.getenv("RPS_QUERY", "")
ERROR_QUERY = os.getenv("ERROR_QUERY", "")
LATENCY_QUERY = os.getenv("LATENCY_QUERY", "")

INTERVAL = float(os.getenv("INTERVAL", "15"))
START_WEIGHT = int(os.getenv("START_WEIGHT", "10"))
STEP = int(os.getenv("STEP", "20"))
MIN_RPS = float(os.getenv("MIN_RPS", "0.5"))
MAX_ERROR_RATE = float(os.getenv("MAX_ERROR_RATE", "0.05"))
MAX_LATENCY = float(os.getenv("MAX_LATENCY", "0.5"))
BAD_CHECKS = int(os.getenv("BAD_CHECKS", "2"))

NAMESPACE = os.getenv("NAMESPACE", "shop")
ROUTE_NAME = os.getenv("ROUTE_NAME", "shop-api")
ROUTE_API = os.getenv("ROUTE_API", "gateway.networking.k8s.io/v1")
V1_BACKEND = os.getenv("V1_BACKEND", "shop-api-v1")
V2_BACKEND = os.getenv("V2_BACKEND", "shop-api-v2")
METRICS_PORT = int(os.getenv("METRICS_PORT", "9100"))

SA_DIR = "/var/run/secrets/kubernetes.io/serviceaccount"

# Что отдаём на /metrics: текущий вес v2 — для графиков доли трафика.
current_weight = 0


def log(message):
    print(f"[{time.strftime('%H:%M:%S')}] {message}", flush=True)


def decide(weight, rps, error_rate, latency, bad_checks=0):
    """Правило гейта: возвращает новый вес v2, причину и число плохих проверок подряд."""
    if weight <= 0 or weight >= 100:
        return weight, "релиз завершён", 0
    # Нет данных — не повод ни докатывать, ни откатывать.
    if rps is None or rps < MIN_RPS or error_rate is None or latency is None:
        return weight, "мало данных, держим вес", bad_checks

    problem = None
    if error_rate > MAX_ERROR_RATE:
        problem = f"ошибки {error_rate:.1%} > {MAX_ERROR_RATE:.1%}"
    elif latency > MAX_LATENCY:
        problem = f"задержка {latency:.3f}s > {MAX_LATENCY:.3f}s"
    if problem is None:
        return min(weight + STEP, 100), "v2 здорова, поднимаем долю", 0

    # Один всплеск не повод откатывать релиз: ждём подтверждения следующей проверкой.
    bad_checks += 1
    if bad_checks < BAD_CHECKS:
        return weight, f"{problem}, ждём подтверждения", bad_checks
    return 0, f"{problem}, откат", bad_checks


def prom_query(query):
    """Значение instant-запроса; None, если данных нет или Prometheus недоступен."""
    url = f"{PROM_URL}/api/v1/query?" + urllib.parse.urlencode({"query": query})
    try:
        with urllib.request.urlopen(url, timeout=5) as resp:
            result = json.load(resp)["data"]["result"]
        value = float(result[0]["value"][1])
    except (OSError, ValueError, KeyError, IndexError) as err:
        if not isinstance(err, IndexError):
            log(f"ошибка запроса к Prometheus: {err}")
        return None
    return None if math.isnan(value) else value


def kube_request(method, path, body=None):
    with open(f"{SA_DIR}/token") as f:
        token = f.read()
    host = os.environ["KUBERNETES_SERVICE_HOST"]
    port = os.environ["KUBERNETES_SERVICE_PORT"]
    req = urllib.request.Request(
        f"https://{host}:{port}{path}",
        method=method,
        data=json.dumps(body).encode() if body is not None else None,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/merge-patch+json",
        },
    )
    ctx = ssl.create_default_context(cafile=f"{SA_DIR}/ca.crt")
    with urllib.request.urlopen(req, timeout=5, context=ctx) as resp:
        return json.load(resp)


def apply_weights(rules, weight):
    """Проставляет веса v1/v2 во всех backendRefs маршрута."""
    weights = {V1_BACKEND: 100 - weight, V2_BACKEND: weight}
    for rule in rules:
        for backend in rule.get("backendRefs", []):
            if backend["name"] in weights:
                backend["weight"] = weights[backend["name"]]
    return rules


def set_weight(weight):
    global current_weight
    path = f"/apis/{ROUTE_API}/namespaces/{NAMESPACE}/httproutes/{ROUTE_NAME}"
    # merge-patch заменяет массив целиком, поэтому сначала читаем текущие правила.
    rules = kube_request("GET", path)["spec"]["rules"]
    kube_request("PATCH", path, {"spec": {"rules": apply_weights(rules, weight)}})
    current_weight = weight


class MetricsHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = f"gate_v2_weight {current_weight}\n".encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4")
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


def main():
    server = http.server.ThreadingHTTPServer(("", METRICS_PORT), MetricsHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()

    set_weight(START_WEIGHT)
    log(f"старт: v2={START_WEIGHT}%")

    bad_checks = 0
    while True:
        time.sleep(INTERVAL)
        rps = prom_query(RPS_QUERY)
        error_rate = prom_query(ERROR_QUERY)
        latency = prom_query(LATENCY_QUERY)

        new_weight, reason, bad_checks = decide(current_weight, rps, error_rate, latency, bad_checks)
        log(f"v2={current_weight}% rps={rps} errors={error_rate} p95={latency} -> {new_weight}%: {reason}")
        if new_weight != current_weight:
            try:
                set_weight(new_weight)
            except OSError as err:
                log(f"не удалось обновить HTTPRoute: {err}")


if __name__ == "__main__":
    main()
